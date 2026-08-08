package user

import (
	"context"
	"sort"
	"strings"
	"time"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// groupServer identifica um chat de grupo. Como lidServer, é a única forma de
// distinguir em tempo de execução — JID de grupo e de contato são o mesmo
// tipo Go.
const groupServer = "@g.us"

// pnServer é o servidor de telefone. Como groupServer e lidServer, é a única
// forma de distinguir os espaços de identidade em tempo de execução.
const pnServer = "@s.whatsapp.net"

const (
	// LimitPadrao é quantas conversas saem quando o cliente não pede um
	// tamanho. Cinquenta cabe numa tela e evita que o padrão da rota seja
	// "traga tudo" — medido em 728 chats numa sessão real.
	LimitPadrao = 50

	// LimitMaximo é o teto por página. Existe para que um cliente não
	// transforme a paginação em "traga tudo" passando um número grande.
	LimitMaximo = 500
)

// ListChatsUseCase monta a lista de conversas ordenada pela última interação.
//
// Junta três fontes: o histórico local (quando cada conversa teve a última
// mensagem), o roster (o nome de cada contato) e os grupos (o nome de cada
// grupo). Nenhuma delas sozinha responde "com quem eu falei por último".
//
// NÃO devolve mensagens. A lista existe para ordenar conversas; trazer
// conteúdo a tornaria cara sem torná-la mais útil.
type ListChatsUseCase struct {
	activity appport.ChatActivityReader
	contacts appport.ContactDirectory
	groups   appport.GroupDirectory
	logger   appport.Logger
}

// NewListChatsUseCase cria o use case com as portas injetadas.
func NewListChatsUseCase(
	ar appport.ChatActivityReader,
	cd appport.ContactDirectory,
	gd appport.GroupDirectory,
	logger appport.Logger,
) *ListChatsUseCase {
	return &ListChatsUseCase{activity: ar, contacts: cd, groups: gd, logger: logger}
}

// Execute devolve uma página da lista, ordenada da interação mais recente
// para a mais antiga.
//
// limit <= 0 vira LimitPadrao; acima de LimitMaximo é cortado. offset
// negativo vira zero. Entrada fora de faixa é corrigida, e não recusada: a
// alternativa seria um 400 para quem pediu `limit=0` querendo dizer "o
// padrão".
func (uc *ListChatsUseCase) Execute(ctx context.Context, userID string, limit, offset int) (*domain.ChatListPage, error) {
	limit, offset = normalizarPaginacao(limit, offset)

	bruto, err := uc.activity.GetLastActivityByUser(ctx, userID)
	if err != nil {
		uc.logger.Error(ctx, "nao foi possivel ler a atividade por chat", "error", err, "user_id", userID)
		return nil, err
	}
	atividade := normalizeToLID(ctx, uc.contacts, uc.logger, userID, bruto)

	// As duas tabelas de nome sao aliasadas para o espaco @lid porque e' nele
	// que as chaves de atividade chegam depois de normalizeToLID.
	nomesContato := aliasarParaLID(ctx, uc.contacts, uc.logger, userID, uc.nomesDeContato(ctx, userID))
	nomesGrupo := uc.nomesDeGrupo(ctx, userID)
	nomesHistorico := aliasarParaLID(ctx, uc.contacts, uc.logger, userID, uc.nomesDoHistorico(ctx, userID))

	chats := make([]domain.ChatSummary, 0, len(atividade))
	for jid, quando := range atividade {
		chats = append(chats, montarResumo(jid, quando, nomesContato, nomesGrupo, nomesHistorico))
	}

	ordenar(chats)

	total := len(chats)
	uc.logger.Info(ctx, "lista de conversas montada",
		"user_id", userID, "total", total, "limit", limit, "offset", offset)

	return &domain.ChatListPage{
		Chats:  fatiar(chats, limit, offset),
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}, nil
}

func normalizarPaginacao(limit, offset int) (int, int) {
	if limit <= 0 {
		limit = LimitPadrao
	}
	if limit > LimitMaximo {
		limit = LimitMaximo
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

// nomesDeContato e nomesDeGrupo DEGRADAM: sem roster ou sem grupos a lista
// ainda vale — ordenar conversas é o serviço principal, e o nome é
// enriquecimento. Falhar a rota inteira porque o nome de um grupo não veio
// tornaria a lista indisponível justamente quando a rede está ruim.
func (uc *ListChatsUseCase) nomesDeContato(ctx context.Context, userID string) map[string]domain.ContactName {
	nomes, err := uc.contacts.ContactNames(ctx, userID)
	if err != nil {
		uc.logger.Warn(ctx, "roster indisponivel; conversas sairao sem nome de contato", "error", err, "user_id", userID)
		return nil
	}
	// Rechaveado para string para que aliasarParaLID sirva as duas tabelas
	// de nome sem duplicar a logica de alias.
	out := make(map[string]domain.ContactName, len(nomes))
	for jid, c := range nomes {
		out[string(jid)] = c
	}
	return out
}

// nomesDoHistorico é a fonte que a F84 destravou: o pushName que o WhatsApp
// manda junto de cada mensagem. Na prática é a mais completa das três — o
// roster só conhece quem está na agenda, e para `@lid` está vazio na maioria
// dos casos.
//
// A chave vem do chat_jid COMO ELE ESTÁ no histórico, porque é assim que a
// consulta agrupa. Quem reconcilia isso com o espaço `@lid` das chaves de
// atividade é aliasarParaLID, no Execute.
//
// A versão anterior deste comentário afirmava que montarResumo "tenta as
// duas formas". Não tentava — e o resultado foi a lista aproveitar 11 dos
// 480 chats enquanto 7.273 mensagens tinham nome gravado.
func (uc *ListChatsUseCase) nomesDoHistorico(ctx context.Context, userID string) map[string]string {
	nomes, err := uc.activity.GetChatPushNames(ctx, userID)
	if err != nil {
		uc.logger.Warn(ctx, "pushNames do historico indisponiveis; conversas caem para o roster", "error", err, "user_id", userID)
		return nil
	}
	return nomes
}

func (uc *ListChatsUseCase) nomesDeGrupo(ctx context.Context, userID string) map[domain.JID]string {
	nomes, err := uc.groups.GroupNames(ctx, userID)
	if err != nil {
		uc.logger.Warn(ctx, "grupos indisponiveis; conversas de grupo sairao sem nome", "error", err, "user_id", userID)
		return nil
	}
	return nomes
}

// aliasarParaLID acrescenta, a um mapa de consulta chaveado por JID, uma
// entrada extra sob o LID equivalente de cada chave `@s.whatsapp.net`.
//
// Existe porque a lista NORMALIZA as chaves de atividade para `@lid`
// (normalizeToLID) enquanto as tabelas de consulta — roster e pushName do
// histórico — vivem no espaço em que cada dado foi gravado. Sem o alias, os
// dois lados nunca se encontram: medido em produção, 120 dos 135 chats com
// pushName tinham chave `@s.whatsapp.net`, e a lista aproveitava 11 de 480.
//
// Aliasa em vez de REESCREVER: a chave original continua valendo, porque
// nem toda entrada de atividade é normalizada (PN sem LID conhecido mantém
// o PN, ver normalizeToLID).
func aliasarParaLID[V any](
	ctx context.Context,
	contacts appport.ContactDirectory,
	logger appport.Logger,
	userID string,
	tabela map[string]V,
) map[string]V {
	if len(tabela) == 0 {
		return tabela
	}
	pns := make([]domain.JID, 0, len(tabela))
	for k := range tabela {
		if strings.HasSuffix(k, pnServer) {
			pns = append(pns, domain.JID(k))
		}
	}
	if len(pns) == 0 {
		return tabela
	}

	resolvidos, err := contacts.GetManyLIDsForPNs(ctx, userID, pns)
	if err != nil {
		// Best-effort, como em normalizeToLID: sem o mapa, a tabela original
		// continua valendo para as chaves que já batem.
		logger.Warn(ctx, "nao foi possivel aliasar a tabela de nomes para LID", "error", err, "user_id", userID)
		return tabela
	}

	out := make(map[string]V, len(tabela)+len(resolvidos))
	for k, v := range tabela {
		out[k] = v
	}
	for pn, lid := range resolvidos {
		if lid == "" {
			continue
		}
		// Não sobrescreve: uma entrada que já veio chaveada por LID é mais
		// direta que a derivada de um PN.
		if _, existe := out[string(lid)]; existe {
			continue
		}
		if v, ok := tabela[string(pn)]; ok {
			out[string(lid)] = v
		}
	}
	return out
}

// montarResumo decide o nome de UMA conversa.
//
// Grupo e contato buscam em mapas diferentes, e o sufixo do JID é o que
// decide qual — o roster não tem entradas `@g.us`, então procurar um grupo
// nele devolveria vazio sem erro nenhum.
func montarResumo(
	jid string,
	quando time.Time,
	contatos map[string]domain.ContactName,
	grupos map[domain.JID]string,
	historico map[string]string,
) domain.ChatSummary {
	resumo := domain.ChatSummary{
		JID:          jid,
		LastActivity: quando,
		IsGroup:      strings.HasSuffix(jid, groupServer),
	}
	if resumo.IsGroup {
		resumo.Name = grupos[domain.JID(jid)]
		return resumo
	}

	// O roster vem primeiro por ser o nome que QUEM CONSULTA escolheu: se a
	// pessoa está na agenda como "Maria Contadora", é isso que se espera ver,
	// e não o pushName que ela escolheu para si.
	if c, ok := contatos[jid]; ok {
		resumo.PushName = c.PushName
		resumo.FullName = c.FullName
		resumo.BusinessName = c.BusinessName
		resumo.Name = c.Melhor()
	}

	// O histórico entra quando o roster não soube — que é a maioria dos casos
	// para `@lid` (F84). Nunca sobrescreve um nome que o roster deu: isso
	// inverteria a preferência acima.
	if resumo.Name == "" {
		if nome := historico[jid]; nome != "" {
			resumo.Name = nome
			resumo.PushName = nome
		}
	}
	return resumo
}

// ordenar coloca a interação mais recente primeiro.
//
// O desempate por JID não é cosmético: sem ele, dois chats com o mesmo
// timestamp trocariam de posição entre chamadas (sort.Slice não é estável e a
// ordem de iteração de um map em Go é aleatória), e um cliente paginando
// veria a mesma conversa duas vezes ou nenhuma.
func ordenar(chats []domain.ChatSummary) {
	sort.Slice(chats, func(i, j int) bool {
		if chats[i].LastActivity.Equal(chats[j].LastActivity) {
			return chats[i].JID < chats[j].JID
		}
		return chats[i].LastActivity.After(chats[j].LastActivity)
	})
}

// fatiar aplica limit/offset sem entrar em pânico com offset além do fim,
// que é o pedido normal de um cliente que chegou ao último página.
func fatiar(chats []domain.ChatSummary, limit, offset int) []domain.ChatSummary {
	if offset >= len(chats) {
		return []domain.ChatSummary{}
	}
	fim := offset + limit
	if fim > len(chats) {
		fim = len(chats)
	}
	return chats[offset:fim]
}
