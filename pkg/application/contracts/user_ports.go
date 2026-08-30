package port

import (
	"context"
	"time"

	"wa-api/pkg/domain"
)

// As consultas sobre contatos vivem em TRÊS portas, e a fronteira entre elas foi
// medida e não escolhida (decisão 82).
//
// Contei quem usa o quê: dos nove casos de uso que consumiam a interface única,
// SETE precisavam de um único método. Só `get_user_profile` usava cinco — e usa,
// porque é o endpoint consolidado que junta identidade, avatar e metadados de
// propósito.
//
// Uma interface de oito métodos obrigava cada um desses sete a declarar
// dependência de sete capacidades que não toca, e obrigava qualquer transporte
// novo a implementar as oito para satisfazer qualquer uma.

// LIDResolver e' a visao de UM metodo da traducao LID->telefone, para quem
// precisa SO' disso. Veio da feature/wa-noise, e sobrevive a divisao da decisao
// 82 sem atrito: qualquer IdentityResolver a satisfaz, entao nenhum adaptador
// muda.
//
// Os dois ramos fizeram o MESMO movimento — estreitar a dependencia — por
// caminhos diferentes: aqui a interface gorda foi PARTIDA por medicao de uso;
// la' foi acrescentada uma vista estreita ao lado dela. Guardar as duas custa
// quatro linhas e nao obriga ninguem a migrar.
type LIDResolver interface {
	// GetPNForLID resolves the phone JID of a @lid. An unknown mapping is an
	// empty JID with a NIL error — absence is an answer, not a failure — so a
	// caller that only checks err would silently treat "unknown" as "resolved
	// to the empty string".
	GetPNForLID(ctx context.Context, txtID string, lid domain.JID) (domain.JID, error)
}

// IdentityResolver responde QUEM é alguém, nas duas direções da identidade dupla.
type IdentityResolver interface {
	SessionGuard

	// IsOnWhatsApp verifica quais dos telefones informados têm conta.
	IsOnWhatsApp(ctx context.Context, txtID string, phones []string) ([]domain.WhatsAppCheck, error)

	// GetLIDForPN resolve o LID correspondente a um número de telefone.
	GetLIDForPN(ctx context.Context, txtID string, jid domain.JID) (domain.JID, error)

	// GetPNForLID resolve o número de telefone correspondente a um LID — a
	// direção INVERSA de GetLIDForPN.
	//
	// As duas existem porque a identidade é dupla e o chamador nem sempre sabe
	// qual das duas recebeu: PN e LID são o mesmo tipo Go, distintos só pelo
	// Server (`@s.whatsapp.net` vs `@lid`) em tempo de execução. Quem aceita
	// "um identificador qualquer" precisa poder ir nos dois sentidos.
	//
	// LID sem mapeamento conhecido devolve JID vazia SEM erro, como
	// GetLIDForPN: ausência de mapeamento é resposta, não falha.
	GetPNForLID(ctx context.Context, txtID string, lid domain.JID) (domain.JID, error)

	// GetManyLIDsForPNs resolve em lote o LID correspondente a cada JID de
	// telefone (`@s.whatsapp.net`) informado — usado pra normalizar
	// last-activity (message_history, majoritariamente PN) pro mesmo espaço
	// de identidade de GetAllContacts (whatsmeow_contacts, majoritariamente
	// @lid no WhatsApp Multi-Device atual). PN sem mapeamento conhecido fica
	// AUSENTE do mapa devolvido (não é erro — o caller mantém o PN original
	// como fallback).
	GetManyLIDsForPNs(ctx context.Context, txtID string, jids []domain.JID) (map[domain.JID]domain.JID, error)
}

// AvatarReader lê a foto de perfil.
type AvatarReader interface {
	SessionGuard

	// GetProfilePicture devolve o avatar de um contato, ou nil se não há.
	GetProfilePicture(ctx context.Context, txtID string, target domain.JID, preview bool) (*domain.AvatarInfo, error)
}

// ContactRoster lê a agenda da sessão.
type ContactRoster interface {
	SessionGuard

	// GetAllContacts devolve a agenda da sessão e a contagem, que o use
	// case usa para logar.
	//
	// A ORDEM é responsabilidade do adaptador e tem de ser DETERMINÍSTICA:
	// as duas implementações leem de um mapa, e a iteração de mapa em Go é
	// aleatória por desenho — sem ordenar, a mesma agenda sairia numa ordem
	// diferente a cada chamada.
	GetAllContacts(ctx context.Context, txtID string) ([]domain.Contact, int, error)

	// ContactNames devolve só os NOMES do roster, indexados por JID.
	//
	// Existe ao lado de GetAllContacts porque quem precisa CASAR nomes por
	// JID quer o índice pronto, e não uma lista para percorrer por cada
	// consulta.
	ContactNames(ctx context.Context, txtID string) (map[domain.JID]domain.ContactName, error)

	// GetUserInfo devolve os metadados dos JIDs informados, na MESMA ORDEM
	// em que foram pedidos.
	//
	// Era `any` até a migração da família de utilizadores, e isso significava
	// que o MOTOR decidia a forma do JSON: o adaptador noise devolvia o
	// map[types.JID]types.UserInfo do SDK e o headless um
	// map[JID]ContactName — dois corpos diferentes para a mesma rota, nenhum
	// deles declarado em lado nenhum.
	GetUserInfo(ctx context.Context, txtID string, jids []domain.JID) ([]domain.UserInfo, error)
}

// ContactDirectory é a composição das três, para o adaptador que satisfaz todas
// declarar isso numa linha. Um caso de uso deve pedir a parte que usa.
type ContactDirectory interface {
	IdentityResolver
	AvatarReader
	ContactRoster
}

// ChatActivityReader expõe o histórico de atividade por chat já persistido
// localmente (message_history) — em particular o backfill automático do
// HistorySync pós-pareamento. Não estende SessionGuard: é leitura de banco
// local, não chamada ao noise, então não exige sessão noise ativa
// (funciona mesmo com a sessão em standby).
type ChatActivityReader interface {
	// GetLastActivityByUser devolve, por JID de chat (string bruta — pode
	// incluir grupos/broadcasts, filtragem é do caller), o timestamp da
	// mensagem mais recente já persistida.
	GetLastActivityByUser(ctx context.Context, userID string) (map[string]time.Time, error)

	// GetChatPushNames devolve, por chat, o pushName mais recente que chegou
	// naquela conversa.
	//
	// É a fonte de nome mais completa que temos: o WhatsApp manda o pushName
	// junto de CADA mensagem, enquanto o roster só conhece quem está na
	// agenda — e para identidades `@lid` está vazio na maioria dos casos
	// (F84). Como GetLastActivityByUser, é leitura de banco local e não
	// exige sessão ativa.
	GetChatPushNames(ctx context.Context, userID string) (map[string]string, error)
}

// BlocklistManager expõe a lista de bloqueados.
type BlocklistManager interface {
	SessionGuard

	// GetBlocklist devolve a lista atual de bloqueados.
	GetBlocklist(ctx context.Context, txtID string) (domain.Blocklist, error)

	// UpdateBlocklist bloqueia (block=true) ou desbloqueia um alvo.
	//
	// A resolução do JID pedido para o JID que a lista de bloqueio de fato
	// usa — normalização de servidor legado, e tradução de LID para número
	// de telefone — é do adapter: é regra do SDK, não do domínio. Por isso o
	// resultado devolve os dois JIDs, e não só um.
	UpdateBlocklist(ctx context.Context, txtID string, target domain.JID, block bool) (domain.BlocklistUpdate, error)
}

// PrivacyManager expõe as configurações de privacidade da sessão.
type PrivacyManager interface {
	SessionGuard

	// GetPrivacySettings devolve as configurações atuais.
	GetPrivacySettings(ctx context.Context, txtID string) (domain.PrivacySettings, error)

	// SetPrivacySetting altera uma configuração e devolve o estado
	// RESULTANTE de todas elas — é o que o servidor responde, e devolver só
	// a que mudou obrigaria o cliente a uma segunda chamada para saber se
	// alguma outra foi arrastada. A validação de name e value é do domínio
	// (domain.ValidatePrivacySetting) e acontece antes.
	SetPrivacySetting(ctx context.Context, txtID, name, value string) (domain.PrivacySettings, error)
}
