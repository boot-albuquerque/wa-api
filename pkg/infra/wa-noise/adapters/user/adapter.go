package user

import (
	"context"
	waclient "wa-api/pkg/infra/wa-noise/client"
	wajid "wa-api/pkg/infra/wa-noise/mapping/jid"
	wasession "wa-api/pkg/infra/wa-noise/runtime/session"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
)

// UserAdapter implementa ContactDirectory, BlocklistManager e PrivacyManager
// sobre o clientManager.
type UserAdapter struct {
	*wasession.SessionGuardAdapter
}

// NewUserAdapter cria o adapter com a função de lookup.
func NewUserAdapter(getClient waclient.Getter) *UserAdapter {
	return &UserAdapter{SessionGuardAdapter: wasession.NewSessionGuardAdapter(getClient)}
}

// IsOnWhatsApp verifica quais dos telefones informados têm conta.
func (a *UserAdapter) IsOnWhatsApp(ctx context.Context, txtID string, phones []string) ([]domain.WhatsAppCheck, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return nil, apperr.New(codeUserSessionUnavailable, apperr.CategoryValidation,
			"no active session for user", false, err)
	}
	resp, err := client.IsOnWhatsApp(ctx, phones)
	if err != nil {
		return nil, apperr.New("user_is_on_whatsapp_failed", apperr.CategoryInternal,
			"failed to check whether phones are on WhatsApp", true, err)
	}

	var out []domain.WhatsAppCheck
	for _, item := range resp {
		verifiedName := ""
		if item.VerifiedName != nil {
			verifiedName = item.VerifiedName.Details.GetVerifiedName()
		}
		out = append(out, domain.WhatsAppCheck{
			Query:        item.Query,
			IsIn:         item.IsIn,
			JID:          item.JID.String(),
			VerifiedName: verifiedName,
		})
	}
	return out, nil
}

// GetUserInfo devolve os metadados dos JIDs informados.
func (a *UserAdapter) GetUserInfo(ctx context.Context, txtID string, jids []domain.JID) (any, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return nil, apperr.New(codeUserSessionUnavailable, apperr.CategoryValidation,
			"no active session for user", false, err)
	}
	parsed, err := wajid.ToJIDs(jids)
	if err != nil {
		// Este erro so' dispara quando os JIDs vieram malformados NO PEDIDO —
		// e' entrada do chamador, nao falha nossa. Estava como
		// CategoryInternal e Retryable=true, o que e' errado nas duas pontas:
		// Category.HTTPStatus() mapeia Internal para 500, e o doc de
		// AppError.Retryable diz que erro de validacao nunca e' retentavel,
		// porque repetir a mesma entrada da' o mesmo resultado.
		//
		// A troca ainda nao muda nada observavel: nada chama HTTPStatus() hoje
		// (ver apperr/codes.go). Muda no dia em que o boundary for ligado, que
		// e' exatamente quando ninguem lembraria de revisitar isto.
		return nil, apperr.New(codeUserInfoTargetsInvalid, apperr.CategoryValidation,
			"failed to resolve user info targets", false, err)
	}
	return client.GetUserInfo(ctx, parsed)
}

// GetAllContacts devolve a agenda da sessão e a contagem.
func (a *UserAdapter) GetAllContacts(ctx context.Context, txtID string) (any, int, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return nil, 0, apperr.New(codeUserSessionUnavailable, apperr.CategoryValidation,
			"no active session for user", false, err)
	}
	contacts, err := client.Store().Contacts.GetAllContacts(ctx)
	if err != nil {
		return nil, 0, err
	}
	return contacts, len(contacts), nil
}

// GetLIDForPN resolve o LID correspondente a um número de telefone.
func (a *UserAdapter) GetLIDForPN(ctx context.Context, txtID string, jid domain.JID) (domain.JID, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return "", err
	}
	parsed, err := wajid.ToJID(jid)
	if err != nil {
		return "", err
	}
	lid, err := client.Store().LIDs.GetLIDForPN(ctx, parsed)
	if err != nil {
		return "", err
	}
	if lid.IsEmpty() {
		return "", nil
	}
	return domain.JID(lid.String()), nil
}

// GetPNForLID resolve o telefone correspondente a um LID.
//
// Espelha GetLIDForPN, inclusive no contrato de ausência: mapeamento
// desconhecido devolve JID vazia e erro nil. O store recusa a chamada com um
// JID que não seja @lid (sqlstore/lidmap.go:125), então passar um PN aqui é
// erro, não silêncio — e é o use case que decide a direção.
func (a *UserAdapter) GetPNForLID(ctx context.Context, txtID string, lid domain.JID) (domain.JID, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return "", err
	}
	parsed, err := wajid.ToJID(lid)
	if err != nil {
		return "", err
	}
	pn, err := client.Store().LIDs.GetPNForLID(ctx, parsed)
	if err != nil {
		return "", err
	}
	if pn.IsEmpty() {
		return "", nil
	}
	return domain.JID(pn.String()), nil
}

// GetManyLIDsForPNs resolve em lote (1 chamada ao store local, não 1 por
// JID) o LID de cada telefone informado.
//
// A ORIENTAÇÃO DO MAPA IMPORTA (F65 em HOUSEKEEP.md).
// CachedLIDMap.GetManyLIDsForPNs devolve map[PN]LID — `result[pn] = lid`
// (sqlstore/lidmap.go:148). A chave do range é o PN; o valor, o LID.
//
// Até a F65 este laço estava escrito como `for lid, pn := range resolved`,
// invertendo o mapa de saída. O dublê de teste do pacote estava invertido do
// mesmo jeito, então os dois combinavam e o teste ficava verde enquanto a
// normalização LID↔PN não acontecia em produção. Medido no ambiente real:
// 419 dos 421 telefones não normalizados TINHAM mapeamento no store.
//
// PN e LID são o mesmo tipo Go (types.JID), distintos só pelo Server em
// tempo de execução — o compilador não pode ajudar aqui.
func (a *UserAdapter) GetManyLIDsForPNs(ctx context.Context, txtID string, jids []domain.JID) (map[domain.JID]domain.JID, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return nil, err
	}
	parsed, err := wajid.ToJIDs(jids)
	if err != nil {
		return nil, err
	}
	resolved, err := client.Store().LIDs.GetManyLIDsForPNs(ctx, parsed)
	if err != nil {
		return nil, err
	}
	out := make(map[domain.JID]domain.JID, len(resolved))
	for pn, lid := range resolved {
		if pn.IsEmpty() || lid.IsEmpty() {
			continue
		}
		out[domain.JID(pn.String())] = domain.JID(lid.String())
	}
	return out, nil
}

// Verificações em tempo de compilação de que o adapter implementa as portas.
var (
	_ appport.ContactDirectory = (*UserAdapter)(nil)
	_ appport.BlocklistManager = (*UserAdapter)(nil)
	_ appport.PrivacyManager   = (*UserAdapter)(nil)
)

// ContactNames devolve o roster tipado por JID.
//
// Mesma leitura de GetAllContacts, com a conversão do tipo do SDK feita AQUI,
// que é onde ela pertence: o adapter existe para que o vendor pare nesta
// fronteira. GetAllContacts continua devolvendo `any` para quem só repassa o
// bloco cru ao cliente.
func (a *UserAdapter) ContactNames(ctx context.Context, txtID string) (map[domain.JID]domain.ContactName, error) {
	client, err := a.Client(txtID)
	if err != nil {
		return nil, apperr.New(codeUserSessionUnavailable, apperr.CategoryValidation,
			"no active session for user", false, err)
	}
	contacts, err := client.Store().Contacts.GetAllContacts(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[domain.JID]domain.ContactName, len(contacts))
	for jid, info := range contacts {
		out[domain.JID(jid.String())] = domain.ContactName{
			FullName:     info.FullName,
			FirstName:    info.FirstName,
			PushName:     info.PushName,
			BusinessName: info.BusinessName,
		}
	}
	return out, nil
}
