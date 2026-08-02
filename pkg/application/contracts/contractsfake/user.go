package contractsfake

import (
	"context"

	port "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// --- ContactDirectory --------------------------------------------------

// ContactDirectoryIsOnWhatsAppCall é uma chamada a IsOnWhatsApp.
type ContactDirectoryIsOnWhatsAppCall struct {
	Ctx    context.Context
	TxtID  string
	Phones []string
}

// ContactDirectoryGetUserInfoCall é uma chamada a GetUserInfo.
type ContactDirectoryGetUserInfoCall struct {
	Ctx   context.Context
	TxtID string
	JIDs  []domain.JID
}

// ContactDirectoryGetAllContactsCall é uma chamada a GetAllContacts.
type ContactDirectoryGetAllContactsCall struct {
	Ctx   context.Context
	TxtID string
}

// ContactDirectoryGetProfilePictureCall é uma chamada a GetProfilePicture.
type ContactDirectoryGetProfilePictureCall struct {
	Ctx     context.Context
	TxtID   string
	Target  domain.JID
	Preview bool
}

// ContactDirectoryGetLIDForPNCall é uma chamada a GetLIDForPN.
type ContactDirectoryGetLIDForPNCall struct {
	Ctx   context.Context
	TxtID string
	JID   domain.JID
}

// ContactDirectory é o fake de port.ContactDirectory.
type ContactDirectory struct {
	SessionGuard

	IsOnWhatsAppFunc  func(ctx context.Context, txtID string, phones []string) ([]domain.WhatsAppCheck, error)
	IsOnWhatsAppCalls []ContactDirectoryIsOnWhatsAppCall

	GetUserInfoFunc  func(ctx context.Context, txtID string, jids []domain.JID) (any, error)
	GetUserInfoCalls []ContactDirectoryGetUserInfoCall

	GetAllContactsFunc  func(ctx context.Context, txtID string) (any, int, error)
	GetAllContactsCalls []ContactDirectoryGetAllContactsCall

	GetProfilePictureFunc  func(ctx context.Context, txtID string, target domain.JID, preview bool) (*domain.AvatarInfo, error)
	GetProfilePictureCalls []ContactDirectoryGetProfilePictureCall

	GetLIDForPNFunc  func(ctx context.Context, txtID string, jid domain.JID) (domain.JID, error)
	GetLIDForPNCalls []ContactDirectoryGetLIDForPNCall
}

var _ port.ContactDirectory = (*ContactDirectory)(nil)

// IsOnWhatsApp implementa port.ContactDirectory.
func (f *ContactDirectory) IsOnWhatsApp(ctx context.Context, txtID string, phones []string) ([]domain.WhatsAppCheck, error) {
	f.IsOnWhatsAppCalls = append(f.IsOnWhatsAppCalls, ContactDirectoryIsOnWhatsAppCall{Ctx: ctx, TxtID: txtID, Phones: phones})
	if f.IsOnWhatsAppFunc != nil {
		return f.IsOnWhatsAppFunc(ctx, txtID, phones)
	}
	return nil, nil
}

// GetUserInfo implementa port.ContactDirectory.
func (f *ContactDirectory) GetUserInfo(ctx context.Context, txtID string, jids []domain.JID) (any, error) {
	f.GetUserInfoCalls = append(f.GetUserInfoCalls, ContactDirectoryGetUserInfoCall{Ctx: ctx, TxtID: txtID, JIDs: jids})
	if f.GetUserInfoFunc != nil {
		return f.GetUserInfoFunc(ctx, txtID, jids)
	}
	return nil, nil
}

// GetAllContacts implementa port.ContactDirectory.
func (f *ContactDirectory) GetAllContacts(ctx context.Context, txtID string) (any, int, error) {
	f.GetAllContactsCalls = append(f.GetAllContactsCalls, ContactDirectoryGetAllContactsCall{Ctx: ctx, TxtID: txtID})
	if f.GetAllContactsFunc != nil {
		return f.GetAllContactsFunc(ctx, txtID)
	}
	return nil, 0, nil
}

// GetProfilePicture implementa port.ContactDirectory.
func (f *ContactDirectory) GetProfilePicture(ctx context.Context, txtID string, target domain.JID, preview bool) (*domain.AvatarInfo, error) {
	f.GetProfilePictureCalls = append(f.GetProfilePictureCalls, ContactDirectoryGetProfilePictureCall{Ctx: ctx, TxtID: txtID, Target: target, Preview: preview})
	if f.GetProfilePictureFunc != nil {
		return f.GetProfilePictureFunc(ctx, txtID, target, preview)
	}
	return nil, nil
}

// GetLIDForPN implementa port.ContactDirectory.
func (f *ContactDirectory) GetLIDForPN(ctx context.Context, txtID string, jid domain.JID) (domain.JID, error) {
	f.GetLIDForPNCalls = append(f.GetLIDForPNCalls, ContactDirectoryGetLIDForPNCall{Ctx: ctx, TxtID: txtID, JID: jid})
	if f.GetLIDForPNFunc != nil {
		return f.GetLIDForPNFunc(ctx, txtID, jid)
	}
	return "", nil
}

// --- BlocklistManager --------------------------------------------------

// BlocklistManagerGetBlocklistCall é uma chamada a GetBlocklist.
type BlocklistManagerGetBlocklistCall struct {
	Ctx   context.Context
	TxtID string
}

// BlocklistManagerUpdateBlocklistCall é uma chamada a UpdateBlocklist.
type BlocklistManagerUpdateBlocklistCall struct {
	Ctx    context.Context
	TxtID  string
	Target domain.JID
	Block  bool
}

// BlocklistManager é o fake de port.BlocklistManager.
type BlocklistManager struct {
	SessionGuard

	GetBlocklistFunc  func(ctx context.Context, txtID string) (domain.Blocklist, error)
	GetBlocklistCalls []BlocklistManagerGetBlocklistCall

	UpdateBlocklistFunc  func(ctx context.Context, txtID string, target domain.JID, block bool) (domain.BlocklistUpdate, error)
	UpdateBlocklistCalls []BlocklistManagerUpdateBlocklistCall
}

var _ port.BlocklistManager = (*BlocklistManager)(nil)

// GetBlocklist implementa port.BlocklistManager.
func (f *BlocklistManager) GetBlocklist(ctx context.Context, txtID string) (domain.Blocklist, error) {
	f.GetBlocklistCalls = append(f.GetBlocklistCalls, BlocklistManagerGetBlocklistCall{Ctx: ctx, TxtID: txtID})
	if f.GetBlocklistFunc != nil {
		return f.GetBlocklistFunc(ctx, txtID)
	}
	return domain.Blocklist{}, nil
}

// UpdateBlocklist implementa port.BlocklistManager.
func (f *BlocklistManager) UpdateBlocklist(ctx context.Context, txtID string, target domain.JID, block bool) (domain.BlocklistUpdate, error) {
	f.UpdateBlocklistCalls = append(f.UpdateBlocklistCalls, BlocklistManagerUpdateBlocklistCall{Ctx: ctx, TxtID: txtID, Target: target, Block: block})
	if f.UpdateBlocklistFunc != nil {
		return f.UpdateBlocklistFunc(ctx, txtID, target, block)
	}
	return domain.BlocklistUpdate{}, nil
}

// --- PrivacyManager ----------------------------------------------------

// PrivacyManagerGetPrivacySettingsCall é uma chamada a GetPrivacySettings.
type PrivacyManagerGetPrivacySettingsCall struct {
	Ctx   context.Context
	TxtID string
}

// PrivacyManagerSetPrivacySettingCall é uma chamada a SetPrivacySetting.
type PrivacyManagerSetPrivacySettingCall struct {
	Ctx   context.Context
	TxtID string
	Name  string
	Value string
}

// PrivacyManager é o fake de port.PrivacyManager.
type PrivacyManager struct {
	SessionGuard

	GetPrivacySettingsFunc  func(ctx context.Context, txtID string) (any, error)
	GetPrivacySettingsCalls []PrivacyManagerGetPrivacySettingsCall

	SetPrivacySettingFunc  func(ctx context.Context, txtID, name, value string) (any, error)
	SetPrivacySettingCalls []PrivacyManagerSetPrivacySettingCall
}

var _ port.PrivacyManager = (*PrivacyManager)(nil)

// GetPrivacySettings implementa port.PrivacyManager.
func (f *PrivacyManager) GetPrivacySettings(ctx context.Context, txtID string) (any, error) {
	f.GetPrivacySettingsCalls = append(f.GetPrivacySettingsCalls, PrivacyManagerGetPrivacySettingsCall{Ctx: ctx, TxtID: txtID})
	if f.GetPrivacySettingsFunc != nil {
		return f.GetPrivacySettingsFunc(ctx, txtID)
	}
	return nil, nil
}

// SetPrivacySetting implementa port.PrivacyManager.
func (f *PrivacyManager) SetPrivacySetting(ctx context.Context, txtID, name, value string) (any, error) {
	f.SetPrivacySettingCalls = append(f.SetPrivacySettingCalls, PrivacyManagerSetPrivacySettingCall{Ctx: ctx, TxtID: txtID, Name: name, Value: value})
	if f.SetPrivacySettingFunc != nil {
		return f.SetPrivacySettingFunc(ctx, txtID, name, value)
	}
	return nil, nil
}

// --- UserRepository ----------------------------------------------------

// UserRepositoryCreateUserCall é uma chamada a CreateUser.
type UserRepositoryCreateUserCall struct {
	Ctx context.Context
	Rec domain.UserRecord
}

// UserRepositoryUserExistsCall é uma chamada a UserExists.
type UserRepositoryUserExistsCall struct {
	Ctx context.Context
	ID  string
}

// UserRepositoryUpdateUserCall é uma chamada a UpdateUser.
type UserRepositoryUpdateUserCall struct {
	Ctx    context.Context
	ID     string
	Update domain.UserUpdate
}

// UserRepositoryListUsersCall é uma chamada a ListUsers.
type UserRepositoryListUsersCall struct {
	Ctx context.Context
	ID  string
}

// UserRepositoryDeleteUserCall é uma chamada a DeleteUser.
type UserRepositoryDeleteUserCall struct {
	Ctx context.Context
	ID  string
}

// UserRepository é o fake de port.UserRepository.
//
// Atenção aos zero-values dos dois métodos com bool de resultado: sem Func
// configurada, CreateUser devolve created=TRUE e DeleteUser devolve
// deleted=TRUE. É o caminho feliz — e é o que faz o zero-value do fake ser
// útil sem configuração. Para o caminho de conflito/ausência, configure a
// Func.
type UserRepository struct {
	CreateUserFunc  func(ctx context.Context, rec domain.UserRecord) (bool, error)
	CreateUserCalls []UserRepositoryCreateUserCall

	UserExistsFunc  func(ctx context.Context, id string) (bool, error)
	UserExistsCalls []UserRepositoryUserExistsCall

	UpdateUserFunc  func(ctx context.Context, id string, upd domain.UserUpdate) error
	UpdateUserCalls []UserRepositoryUpdateUserCall

	ListUsersFunc  func(ctx context.Context, id string) ([]domain.UserListEntry, error)
	ListUsersCalls []UserRepositoryListUsersCall

	DeleteUserFunc  func(ctx context.Context, id string) (bool, error)
	DeleteUserCalls []UserRepositoryDeleteUserCall
}

var _ port.UserRepository = (*UserRepository)(nil)

// CreateUser implementa port.UserRepository.
func (f *UserRepository) CreateUser(ctx context.Context, rec domain.UserRecord) (bool, error) {
	f.CreateUserCalls = append(f.CreateUserCalls, UserRepositoryCreateUserCall{Ctx: ctx, Rec: rec})
	if f.CreateUserFunc != nil {
		return f.CreateUserFunc(ctx, rec)
	}
	return true, nil
}

// UserExists implementa port.UserRepository.
func (f *UserRepository) UserExists(ctx context.Context, id string) (bool, error) {
	f.UserExistsCalls = append(f.UserExistsCalls, UserRepositoryUserExistsCall{Ctx: ctx, ID: id})
	if f.UserExistsFunc != nil {
		return f.UserExistsFunc(ctx, id)
	}
	return false, nil
}

// UpdateUser implementa port.UserRepository.
func (f *UserRepository) UpdateUser(ctx context.Context, id string, upd domain.UserUpdate) error {
	f.UpdateUserCalls = append(f.UpdateUserCalls, UserRepositoryUpdateUserCall{Ctx: ctx, ID: id, Update: upd})
	if f.UpdateUserFunc != nil {
		return f.UpdateUserFunc(ctx, id, upd)
	}
	return nil
}

// ListUsers implementa port.UserRepository.
func (f *UserRepository) ListUsers(ctx context.Context, id string) ([]domain.UserListEntry, error) {
	f.ListUsersCalls = append(f.ListUsersCalls, UserRepositoryListUsersCall{Ctx: ctx, ID: id})
	if f.ListUsersFunc != nil {
		return f.ListUsersFunc(ctx, id)
	}
	return nil, nil
}

// DeleteUser implementa port.UserRepository.
func (f *UserRepository) DeleteUser(ctx context.Context, id string) (bool, error) {
	f.DeleteUserCalls = append(f.DeleteUserCalls, UserRepositoryDeleteUserCall{Ctx: ctx, ID: id})
	if f.DeleteUserFunc != nil {
		return f.DeleteUserFunc(ctx, id)
	}
	return true, nil
}

// --- DeleteUserProvider ------------------------------------------------

// DeleteUserProviderCheckUserExistsCall é uma chamada a CheckUserExists.
type DeleteUserProviderCheckUserExistsCall struct {
	UserID string
}

// DeleteUserProviderGetUserInfoCall é uma chamada a GetUserInfo.
type DeleteUserProviderGetUserInfoCall struct {
	UserID string
}

// DeleteUserProviderDeleteUserFromDBCall é uma chamada a DeleteUserFromDB.
type DeleteUserProviderDeleteUserFromDBCall struct {
	UserID string
}

// DeleteUserProviderGetS3EnabledCall é uma chamada a GetS3Enabled.
type DeleteUserProviderGetS3EnabledCall struct {
	UserID string
}

// DeleteUserProviderDeleteLocalUserFilesCall é uma chamada a
// DeleteLocalUserFiles.
type DeleteUserProviderDeleteLocalUserFilesCall struct {
	UserID string
	ExPath string
}

// DeleteUserProvider é o fake de port.DeleteUserProvider. É a única porta do
// pacote sem context.Context nas assinaturas — o formato é do upstream.
//
// Zero-value: CheckUserExists devolve true (usuário existe, caminho feliz do
// delete) e GetS3Enabled devolve false.
type DeleteUserProvider struct {
	CheckUserExistsFunc  func(userID string) (bool, error)
	CheckUserExistsCalls []DeleteUserProviderCheckUserExistsCall

	GetUserInfoFunc  func(userID string) (name, jid, token string, err error)
	GetUserInfoCalls []DeleteUserProviderGetUserInfoCall

	DeleteUserFromDBFunc  func(userID string) error
	DeleteUserFromDBCalls []DeleteUserProviderDeleteUserFromDBCall

	GetS3EnabledFunc  func(userID string) (bool, error)
	GetS3EnabledCalls []DeleteUserProviderGetS3EnabledCall

	DeleteLocalUserFilesFunc  func(userID, exPath string) error
	DeleteLocalUserFilesCalls []DeleteUserProviderDeleteLocalUserFilesCall
}

var _ port.DeleteUserProvider = (*DeleteUserProvider)(nil)

// CheckUserExists implementa port.DeleteUserProvider.
func (f *DeleteUserProvider) CheckUserExists(userID string) (bool, error) {
	f.CheckUserExistsCalls = append(f.CheckUserExistsCalls, DeleteUserProviderCheckUserExistsCall{UserID: userID})
	if f.CheckUserExistsFunc != nil {
		return f.CheckUserExistsFunc(userID)
	}
	return true, nil
}

// GetUserInfo implementa port.DeleteUserProvider.
func (f *DeleteUserProvider) GetUserInfo(userID string) (string, string, string, error) {
	f.GetUserInfoCalls = append(f.GetUserInfoCalls, DeleteUserProviderGetUserInfoCall{UserID: userID})
	if f.GetUserInfoFunc != nil {
		return f.GetUserInfoFunc(userID)
	}
	return "", "", "", nil
}

// DeleteUserFromDB implementa port.DeleteUserProvider.
func (f *DeleteUserProvider) DeleteUserFromDB(userID string) error {
	f.DeleteUserFromDBCalls = append(f.DeleteUserFromDBCalls, DeleteUserProviderDeleteUserFromDBCall{UserID: userID})
	if f.DeleteUserFromDBFunc != nil {
		return f.DeleteUserFromDBFunc(userID)
	}
	return nil
}

// GetS3Enabled implementa port.DeleteUserProvider.
func (f *DeleteUserProvider) GetS3Enabled(userID string) (bool, error) {
	f.GetS3EnabledCalls = append(f.GetS3EnabledCalls, DeleteUserProviderGetS3EnabledCall{UserID: userID})
	if f.GetS3EnabledFunc != nil {
		return f.GetS3EnabledFunc(userID)
	}
	return false, nil
}

// DeleteLocalUserFiles implementa port.DeleteUserProvider.
func (f *DeleteUserProvider) DeleteLocalUserFiles(userID, exPath string) error {
	f.DeleteLocalUserFilesCalls = append(f.DeleteLocalUserFilesCalls, DeleteUserProviderDeleteLocalUserFilesCall{UserID: userID, ExPath: exPath})
	if f.DeleteLocalUserFilesFunc != nil {
		return f.DeleteLocalUserFilesFunc(userID, exPath)
	}
	return nil
}
