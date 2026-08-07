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

// Verificações em tempo de compilação de que o adapter implementa as portas.
var (
	_ appport.ContactDirectory = (*UserAdapter)(nil)
	_ appport.BlocklistManager = (*UserAdapter)(nil)
	_ appport.PrivacyManager   = (*UserAdapter)(nil)
)
