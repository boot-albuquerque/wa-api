package whatsmeow

import (
	"context"
	"fmt"
	"time"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"

	wa "go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// UserAdapter implementa ContactDirectory, BlocklistManager e PrivacyManager
// sobre o clientManager.
type UserAdapter struct {
	*SessionGuardAdapter
}

// NewUserAdapter cria o adapter com a função de lookup.
func NewUserAdapter(getClient waClientGetter) *UserAdapter {
	return &UserAdapter{SessionGuardAdapter: NewSessionGuardAdapter(getClient)}
}

func (a *UserAdapter) client(txtID string) (waClient, error) {
	client := a.getClient(txtID)
	if client == nil {
		return nil, ErrNoSession(txtID, nil)
	}
	return client, nil
}

// IsOnWhatsApp verifica quais dos telefones informados têm conta.
func (a *UserAdapter) IsOnWhatsApp(ctx context.Context, txtID string, phones []string) ([]domain.WhatsAppCheck, error) {
	client, err := a.client(txtID)
	if err != nil {
		return nil, err
	}
	resp, err := client.IsOnWhatsApp(ctx, phones)
	if err != nil {
		return nil, err
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
	client, err := a.client(txtID)
	if err != nil {
		return nil, err
	}
	parsed, err := toJIDs(jids)
	if err != nil {
		return nil, err
	}
	return client.GetUserInfo(ctx, parsed)
}

// GetAllContacts devolve a agenda da sessão e a contagem.
func (a *UserAdapter) GetAllContacts(ctx context.Context, txtID string) (any, int, error) {
	client, err := a.client(txtID)
	if err != nil {
		return nil, 0, err
	}
	contacts, err := client.Store().Contacts.GetAllContacts(ctx)
	if err != nil {
		return nil, 0, err
	}
	return contacts, len(contacts), nil
}

// GetProfilePicture devolve o avatar de um contato, ou nil se não há.
func (a *UserAdapter) GetProfilePicture(ctx context.Context, txtID string, target domain.JID, preview bool) (*domain.AvatarInfo, error) {
	client, err := a.client(txtID)
	if err != nil {
		return nil, err
	}
	jid, err := toJID(target)
	if err != nil {
		return nil, err
	}

	// ExistingID vazio: o use case sempre passava "".
	pic, err := client.GetProfilePictureInfo(ctx, jid, &wa.GetProfilePictureParams{
		Preview:    preview,
		ExistingID: "",
	})
	if err != nil {
		return nil, err
	}
	if pic == nil {
		return nil, nil
	}
	return &domain.AvatarInfo{ID: pic.ID, URL: pic.URL}, nil
}

// GetLIDForPN resolve o LID correspondente a um número de telefone.
func (a *UserAdapter) GetLIDForPN(ctx context.Context, txtID string, jid domain.JID) (domain.JID, error) {
	client, err := a.client(txtID)
	if err != nil {
		return "", err
	}
	parsed, err := toJID(jid)
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

// GetBlocklist devolve a lista atual de bloqueados.
func (a *UserAdapter) GetBlocklist(ctx context.Context, txtID string) (domain.Blocklist, error) {
	client, err := a.client(txtID)
	if err != nil {
		return domain.Blocklist{}, err
	}

	ctxWithTimeout, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	blocklist, err := client.GetBlocklist(ctxWithTimeout)
	if err != nil {
		return domain.Blocklist{}, err
	}
	return toDomainBlocklist(blocklist), nil
}

// UpdateBlocklist bloqueia ou desbloqueia um alvo.
func (a *UserAdapter) UpdateBlocklist(ctx context.Context, txtID string, target domain.JID, block bool) (domain.BlocklistUpdate, error) {
	client, err := a.client(txtID)
	if err != nil {
		return domain.BlocklistUpdate{}, err
	}
	parsed, err := toJID(target)
	if err != nil {
		return domain.BlocklistUpdate{}, err
	}
	requested := normalizeBlocklistJID(parsed)

	action := events.BlocklistChangeActionUnblock
	if block {
		action = events.BlocklistChangeActionBlock
	}

	resolved, err := resolveBlocklistPNJID(ctx, client, requested)
	if err != nil {
		return domain.BlocklistUpdate{}, err
	}

	blocklist, err := client.UpdateBlocklist(ctx, resolved, action)
	if err != nil {
		return domain.BlocklistUpdate{}, err
	}

	list := toDomainBlocklist(blocklist)
	return domain.BlocklistUpdate{
		ResolvedJID:  domain.JID(resolved.String()),
		RequestedJID: domain.JID(requested.String()),
		Entries:      list.JIDs,
		DHash:        list.DHash,
	}, nil
}

// toDomainBlocklist converte a lista do SDK, tolerando nil como o use case
// tolerava.
func toDomainBlocklist(blocklist *types.Blocklist) domain.Blocklist {
	out := domain.Blocklist{JIDs: []string{}}
	if blocklist == nil {
		return out
	}
	out.JIDs = make([]string, len(blocklist.JIDs))
	for i, jid := range blocklist.JIDs {
		out.JIDs[i] = jid.String()
	}
	out.DHash = blocklist.DHash
	return out
}

// normalizeBlocklistJID descarta agente/dispositivo e troca o servidor legado
// pelo padrão. Migrado literalmente de block_user.go.
func normalizeBlocklistJID(jid types.JID) types.JID {
	jid = jid.ToNonAD()
	if jid.Server == types.LegacyUserServer {
		jid.Server = types.DefaultUserServer
	}
	return jid
}

// resolveBlocklistPNJID traduz um LID para o número de telefone, que é a
// forma que a lista de bloqueio aceita. Migrado literalmente de block_user.go,
// menos a asserção de tipo `client.(*whatsmeow.Client)`, que existia só
// porque o helper recebia interface{} — aqui o tipo é a interface waClient
// e o Store é acessado pelo método Store().
func resolveBlocklistPNJID(ctx context.Context, client waClient, jid types.JID) (types.JID, error) {
	jid = normalizeBlocklistJID(jid)
	switch jid.Server {
	case types.DefaultUserServer:
		return jid, nil
	case types.HiddenUserServer:
		pn, err := getCachedPNForLID(ctx, client, jid)
		if err != nil {
			return types.JID{}, err
		}
		return normalizeBlocklistJID(pn), nil
	default:
		return types.JID{}, fmt.Errorf("unsupported blocklist JID server %q", jid.Server)
	}
}

func getCachedPNForLID(ctx context.Context, client waClient, jid types.JID) (types.JID, error) {
	store := client.Store()
	if store == nil || store.LIDs == nil {
		return types.JID{}, fmt.Errorf("LID-to-PN mapping store is not available")
	}
	pn, err := store.LIDs.GetPNForLID(ctx, jid)
	if err != nil {
		return types.JID{}, fmt.Errorf("could not resolve phone-number JID for LID %s: %w", jid, err)
	}
	if pn.IsEmpty() {
		return types.JID{}, fmt.Errorf("could not resolve phone-number JID for LID %s", jid)
	}
	return pn, nil
}

// GetPrivacySettings devolve as configurações atuais.
func (a *UserAdapter) GetPrivacySettings(ctx context.Context, txtID string) (any, error) {
	client, err := a.client(txtID)
	if err != nil {
		return nil, err
	}

	ctxWithTimeout, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	return client.TryFetchPrivacySettings(ctxWithTimeout, false)
}

// SetPrivacySetting altera uma configuração de privacidade.
func (a *UserAdapter) SetPrivacySetting(ctx context.Context, txtID, name, value string) (any, error) {
	client, err := a.client(txtID)
	if err != nil {
		return nil, err
	}

	ctxWithTimeout, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	return client.SetPrivacySetting(ctxWithTimeout, types.PrivacySettingType(name), types.PrivacySetting(value))
}

// Verificações em tempo de compilação de que o adapter implementa as portas.
var (
	_ appport.ContactDirectory = (*UserAdapter)(nil)
	_ appport.BlocklistManager = (*UserAdapter)(nil)
	_ appport.PrivacyManager   = (*UserAdapter)(nil)
)
