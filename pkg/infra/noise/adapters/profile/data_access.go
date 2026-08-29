package profile

import (
	"context"
	"wa-api/pkg/infra/noise/client"

	"wa-api/pkg/domain"

	"wa-api/internal/noise"
	"wa-api/internal/noise/protocol/types"
)

// ProfileDataAccess adapta *noise.Client para a interface
// usecase.ProfileDataAccess, permitindo mock em testes unitários.
type ProfileDataAccess struct {
	client *noise.Client
}

// NewProfileDataAccess cria o adapter.
func NewProfileDataAccess(client *noise.Client) *ProfileDataAccess {
	return &ProfileDataAccess{client: client}
}

// NewProfileDataAccessFromInterface aceita a interface client.Client (usada pelos
// adapters) e desembrulha para o tipo concreto que ProfileDataAccess
// requer. Falha com segurança: se o client.Client não for um client.RealClient
// (improvável em produção), devolve ProfileDataAccess com client nil —
// o que reproduz o comportamento anterior de "Store ausente é vazio".
func NewProfileDataAccessFromInterface(c client.Client) *ProfileDataAccess {
	if r, ok := c.(client.RealClient); ok {
		return &ProfileDataAccess{client: r.Client}
	}
	return &ProfileDataAccess{}
}

// PushName retorna o nome público do WhatsApp.
func (d *ProfileDataAccess) PushName() string {
	if d.client != nil && d.client.Store != nil {
		return d.client.Store.PushName
	}
	return ""
}

// OwnJID retorna o JID do próprio dispositivo como domain.JID.
func (d *ProfileDataAccess) OwnJID() (domain.JID, bool) {
	if d.client != nil && d.client.Store != nil && d.client.Store.ID != nil {
		return domain.JID(d.client.Store.ID.ToNonAD().String()), true
	}
	return "", false
}

// toTypesJID converte domain.JID para types.JID (wa-noise).
func toTypesJID(jid domain.JID) (types.JID, error) {
	return types.ParseJID(string(jid))
}

// ProfilePictureURL retorna URL e ID da foto de perfil.
//
// O caminho real desta função exige um *noise.Client inicializado
// pelo SDK (que abre websocket). Esta refatoração fica limitada: o
// adaptador continua a chamar o método concreto porque o SDK não
// oferece uma interface alternativa. Testes diretos desta função
// exercitam apenas o caminho de erro de toTypesJID.
func (d *ProfileDataAccess) ProfilePictureURL(ctx context.Context, jid domain.JID) (string, string, error) {
	tj, err := toTypesJID(jid)
	if err != nil {
		return "", "", err
	}
	if d.client == nil {
		return "", "", nil
	}
	pic, err := d.client.GetProfilePictureInfo(ctx, tj, &noise.GetProfilePictureParams{Preview: false})
	if err != nil || pic == nil {
		return "", "", err
	}
	return pic.URL, pic.ID, nil
}

// ContactInfo retorna nome completo e nome comercial do contato.
func (d *ProfileDataAccess) ContactInfo(ctx context.Context, jid domain.JID) (string, string, error) {
	if d.client == nil || d.client.Store == nil || d.client.Store.Contacts == nil {
		return "", "", nil
	}
	tj, err := toTypesJID(jid)
	if err != nil {
		return "", "", err
	}
	contacts, err := d.client.Store.Contacts.GetAllContacts(ctx)
	if err != nil {
		return "", "", err
	}
	if info, ok := contacts[tj]; ok {
		return info.FullName, info.BusinessName, nil
	}
	return "", "", nil
}

// DeviceInfo lê identidade e estado do aparelho direto do store.
//
// Sem chamada de rede e sem erro: tudo aqui já está em memória. Store
// ausente devolve o zero-value, que reproduz o comportamento das demais
// funções deste adapter ("Store ausente é vazio") em vez de inventar uma
// falha que o chamador não teria como tratar.
func (d *ProfileDataAccess) DeviceInfo() domain.SessionDeviceInfo {
	if d.client == nil {
		return domain.SessionDeviceInfo{}
	}

	// Connected e LoggedIn vêm do cliente, não do store: descrevem a conexão
	// viva, e continuam respondendo mesmo com o store zerado.
	info := domain.SessionDeviceInfo{
		Connected: d.client.IsConnected(),
		LoggedIn:  d.client.IsLoggedIn(),
	}
	if d.client.Store == nil {
		return info
	}

	st := d.client.Store
	info.Platform = st.Platform
	info.RegistrationID = st.RegistrationID
	info.LIDMigrationTimestamp = st.LIDMigrationTimestamp
	info.Initialized = st.Initialized
	// Sem guarda IsEmpty: types.JID.String() de um JID zerado ja devolve "".
	// A guarda que estava aqui era um ramo que nenhum teste conseguia
	// distinguir — medido, nao suposto.
	info.LID = st.LID.String()
	return info
}
