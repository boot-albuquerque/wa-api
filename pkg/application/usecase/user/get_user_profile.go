package user

import (
	"context"
	"strings"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// lidServer é o sufixo que distingue um LID de um telefone. PN e LID são o
// mesmo tipo Go, então esta string é a ÚNICA forma de saber qual dos dois
// chegou — o compilador não ajuda aqui (ver F65).
const lidServer = "@lid"

// GetUserProfileUseCase reúne, num pedido só, tudo o que se sabe sobre um
// contato a partir de QUALQUER uma de suas identidades.
//
// O chamador não precisa saber se tem em mãos um telefone, um JID de telefone
// ou um LID: o use case descobre pelo sufixo e resolve a contraparte. Sem
// isso, quem recebe um `@lid` de um evento e quer o perfil precisa saber de
// antemão qual endpoint chamar.
type GetUserProfileUseCase struct {
	contacts appport.ContactDirectory
	jids     appport.JIDResolver
	logger   appport.Logger
}

// NewGetUserProfileUseCase cria o use case com as portas injetadas.
func NewGetUserProfileUseCase(cd appport.ContactDirectory, jr appport.JIDResolver, logger appport.Logger) *GetUserProfileUseCase {
	return &GetUserProfileUseCase{contacts: cd, jids: jr, logger: logger}
}

// UserProfileResult é o perfil consolidado de um contato.
//
// Sem etiquetas `json`: deixou de ser o formato de fio na migração da família
// de utilizadores. Quem serializa é pkg/presentation/http/dto/user.
type UserProfileResult struct {
	// JID é a identidade de TELEFONE (`@s.whatsapp.net`), e LID a de
	// privacidade (`@lid`). Os dois saem sempre que houver mapeamento,
	// independentemente de qual foi consultado — é o que dispensa o cliente
	// de uma segunda chamada.
	JID string
	LID string

	// Query é o que o cliente pediu, ecoado. Numa resposta que resolve
	// identidades, saber por onde se entrou evita ambiguidade.
	Query string

	// OnWhatsApp responde à pergunta "esse número tem conta?". Falso NÃO é
	// erro: é resposta, e por isso a rota devolve 200 (decisão de contrato).
	// Fica nulo quando a pergunta não pôde ser feita — ver Unavailable.
	OnWhatsApp *bool

	VerifiedName string

	AvatarURL string
	AvatarID  string

	// UserInfo são os metadados que a porta devolve para o alvo — status,
	// dispositivos, id da foto. Era `any` com o tipo do SDK dentro, o que
	// punha o VENDOR na resposta pública; hoje é o tipo de domínio, e o
	// adaptador é que normaliza.

	UserInfo []domain.UserInfo

	// Unavailable lista o que NÃO pôde ser obtido, com o motivo. O perfil
	// degrada em vez de falhar — uma foto indisponível não deve custar o
	// resto —, mas degradar em silêncio é pior que falhar: o cliente veria
	// campos vazios sem saber se é ausência de dado ou falha de rede.
	Unavailable map[string]string
}

// Execute resolve o identificador e monta o perfil.
func (uc *GetUserProfileUseCase) Execute(ctx context.Context, userID string, alvo string) (*UserProfileResult, error) {
	if err := uc.contacts.EnsureSession(ctx, userID); err != nil {
		uc.logger.Warn(ctx, "no wanoise session", "error", err, "user_id", userID)
		return nil, err
	}

	// ResolveJID, e NAO ResolveQualifiedJID: esta rota promete aceitar
	// telefone NU, e o resolvedor estrito rejeita qualquer coisa sem servidor
	// explicito ("JID %q has no server"). O leniente aplica o servidor padrao
	// quando falta o "@" e PRESERVA o servidor quando ele existe
	// (mapping/jid/parse.go:16-27), entao o caminho do @lid continua intacto.
	//
	// Isto foi medido em producao, nao deduzido: com o estrito, /user/profile
	// funcionava para JID e LID e falhava para telefone nu — e o dublê do
	// teste, mais permissivo que a producao, escondia a diferenca.
	jid, err := uc.jids.ResolveJID(ctx, alvo)
	if err != nil {
		uc.logger.Warn(ctx, "identificador invalido", "error", err, "alvo", alvo)
		return nil, err
	}

	res := &UserProfileResult{Query: alvo, Unavailable: map[string]string{}}
	pn := uc.resolverIdentidades(ctx, userID, jid, res)

	// Daqui para baixo, TODA falha degrada: o perfil sai com o que deu certo.
	uc.preencherPresencaNaRede(ctx, userID, res)
	uc.preencherInfo(ctx, userID, pn, res)
	uc.preencherAvatar(ctx, userID, pn, res)

	if len(res.Unavailable) == 0 {
		res.Unavailable = nil
	}
	return res, nil
}

// resolverIdentidades preenche JID e LID a partir do que chegou, e devolve a
// identidade a usar nas consultas seguintes.
//
// Devolve o PN quando ele é conhecido: IsOnWhatsApp só aceita telefone, e as
// demais consultas respondem melhor por ele. Quando só há LID, é o LID que
// segue — pior que o ideal, e melhor que não consultar nada.
func (uc *GetUserProfileUseCase) resolverIdentidades(
	ctx context.Context, userID string, jid domain.JID, res *UserProfileResult,
) domain.JID {
	if strings.HasSuffix(string(jid), lidServer) {
		res.LID = string(jid)
		pn, err := uc.contacts.GetPNForLID(ctx, userID, jid)
		if err != nil {
			uc.logger.Warn(ctx, "nao foi possivel resolver o telefone do LID", "error", err, "lid", string(jid))
			res.Unavailable["jid"] = err.Error()
			return jid
		}
		if pn == "" {
			// Ausência de mapeamento é resposta, não falha — mas o cliente
			// precisa distinguir "não há" de "não perguntei".
			res.Unavailable["jid"] = "sem mapeamento conhecido para este LID"
			return jid
		}
		res.JID = string(pn)
		return pn
	}

	res.JID = string(jid)
	lid, err := uc.contacts.GetLIDForPN(ctx, userID, jid)
	if err != nil {
		uc.logger.Warn(ctx, "nao foi possivel resolver o LID do telefone", "error", err, "jid", string(jid))
		res.Unavailable["lid"] = err.Error()
		return jid
	}
	if lid == "" {
		res.Unavailable["lid"] = "sem mapeamento conhecido para este telefone"
		return jid
	}
	res.LID = string(lid)
	return jid
}

// preencherPresencaNaRede responde "esse número tem conta no WhatsApp?".
//
// Só faz sentido para telefone: IsOnWhatsApp consulta o servidor por número.
// Com apenas um LID em mãos, a pergunta fica sem resposta — e OnWhatsApp
// permanece nulo, distinto de `false`.
func (uc *GetUserProfileUseCase) preencherPresencaNaRede(
	ctx context.Context, userID string, res *UserProfileResult,
) {
	if res.JID == "" {
		res.Unavailable["on_whatsapp"] = "consulta exige telefone; so' ha LID"
		return
	}
	numero := strings.SplitN(strings.TrimSuffix(res.JID, "@s.whatsapp.net"), ":", 2)[0]

	checks, err := uc.contacts.IsOnWhatsApp(ctx, userID, []string{numero})
	if err != nil {
		uc.logger.Warn(ctx, "consulta de presenca na rede falhou", "error", err, "jid", res.JID)
		res.Unavailable["on_whatsapp"] = err.Error()
		return
	}
	if len(checks) == 0 {
		res.Unavailable["on_whatsapp"] = "servidor nao respondeu sobre este numero"
		return
	}
	naRede := checks[0].IsIn
	res.OnWhatsApp = &naRede
	if checks[0].VerifiedName != "" {
		res.VerifiedName = checks[0].VerifiedName
	}
}

func (uc *GetUserProfileUseCase) preencherInfo(
	ctx context.Context, userID string, alvo domain.JID, res *UserProfileResult,
) {
	info, err := uc.contacts.GetUserInfo(ctx, userID, []domain.JID{alvo})
	if err != nil {
		uc.logger.Warn(ctx, "dados de usuario indisponiveis", "error", err, "alvo", string(alvo))
		res.Unavailable["user_info"] = err.Error()
		return
	}
	res.UserInfo = info
}

func (uc *GetUserProfileUseCase) preencherAvatar(
	ctx context.Context, userID string, alvo domain.JID, res *UserProfileResult,
) {
	// preview=false: a rota entrega o perfil COMPLETO, e a foto em tamanho
	// cheio é o que se espera dela. Quem quer miniatura tem /user/avatar.
	avatar, err := uc.contacts.GetProfilePicture(ctx, userID, alvo, false)
	if err != nil {
		// Contato sem foto pública e contato com privacidade fechada chegam
		// aqui como erro tipado (ver a taxonomia de avatar em HOUSEKEEP). Nos
		// dois casos o perfil segue: a ausência da foto é informação, e o
		// motivo fica em Unavailable em vez de virar campo vazio mudo.
		uc.logger.Warn(ctx, "avatar indisponivel", "error", err, "alvo", string(alvo))
		res.Unavailable["avatar"] = err.Error()
		return
	}
	if avatar == nil {
		res.Unavailable["avatar"] = "contato sem foto de perfil publica"
		return
	}
	res.AvatarURL = avatar.URL
	res.AvatarID = avatar.ID
}
