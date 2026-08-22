package user

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"wa-api/pkg/domain/apperr"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	"wa-api/pkg/infra/storage"
)

// EditUserUseCase edita um usuário existente
type EditUserUseCase struct {
	users       appport.UserRepository
	s3Cipher    appport.S3SecretCipher
	republisher appport.UserInfoRepublisher
	logger      appport.Logger
}

// NewEditUserUseCase cria uma nova instância.
//
// s3Cipher encrypts the S3 secret key before it reaches the database,
// following the same pattern AddUserUseCase uses for the HMAC key (F158)
// and for the S3 key (F163). The port is S3-specific because the stored
// type is a TEXT envelope (ADR-0009), not BYTEA.
// republisher drops this user's cached info after a successful write. See
// appport.UserInfoRepublisher and HOUSEKEEP F200/F201: without it the edit
// reached the database and NOTHING else in the process ever saw it.
func NewEditUserUseCase(users appport.UserRepository, s3Cipher appport.S3SecretCipher, republisher appport.UserInfoRepublisher, logger appport.Logger) *EditUserUseCase {
	return &EditUserUseCase{users: users, s3Cipher: s3Cipher, republisher: republisher, logger: logger}
}

// Execute edita um usuário
func (uc *EditUserUseCase) Execute(ctx context.Context, req domain.EditUserRequest) error {
	if req.UserID == "" {
		return apperr.New("missing_user_id", apperr.CategoryValidation, "user ID is required", false, nil)
	}

	// Check if user exists
	exists, err := uc.users.UserExists(ctx, req.UserID)
	if err != nil {
		return fmt.Errorf("database error: %w", err)
	}
	if !exists {
		return apperr.New("user_not_found", apperr.CategoryNotFound, "user not found", false, nil)
	}

	// Validate events if provided
	if req.Events != "" {
		eventList := strings.Split(req.Events, ",")
		for _, event := range eventList {
			event = strings.TrimSpace(event)
			if event == "" {
				continue
			}
			if !isValidEvent(event) {
				return apperr.New(invalidEventTypeCode, apperr.CategoryValidation,
					fmt.Sprintf(invalidEventTypeMsgFmt, event), false, nil)
			}
		}
	}

	// Quais campos mudam é decisão do use case; como isso vira uma instrução
	// SQL é do adapter. Ponteiro nil significa "não informado".
	upd := domain.UserUpdate{}
	if req.Name != "" {
		upd.Name = &req.Name
	}
	if req.Token != "" {
		upd.Token = &req.Token
	}
	if req.Webhook != "" {
		upd.Webhook = &req.Webhook
	}
	if req.Expiration != 0 {
		upd.Expiration = &req.Expiration
	}
	if req.Events != "" {
		upd.Events = &req.Events
	}
	if req.History != 0 {
		upd.History = &req.History
	}
	if req.ProxyConfig != nil {
		proxyURL := ""
		if req.ProxyConfig.Enabled {
			proxyURL = req.ProxyConfig.ProxyURL
		}
		upd.ProxyURL = &proxyURL
		upd.WebhookUseProxy = req.ProxyConfig.WebhookUseProxy
	}
	if req.S3Config != nil {
		// Encrypt the S3 secret before it reaches the database (F163,
		// ADR-0009). The ORDER is the contract: encrypt, then write.
		// A failure returns BEFORE UpdateUser.
		s3Copy := *req.S3Config
		if s3Copy.SecretKey != "" {
			envelope, err := uc.s3Cipher.EncryptS3Secret(s3Copy.SecretKey)
			if err != nil {
				uc.logger.Error(ctx, s3SecretEncryptFailedMsg, "userID", req.UserID, "error", err)
				return fmt.Errorf("%s: %w", s3SecretEncryptFailedMsg, err)
			}
			s3Copy.SecretKey = envelope
		}
		upd.S3 = &s3Copy
	}

	// A ORDEM é o contrato, e está travada em teste: republicar ANTES de a
	// escrita ter sucesso publicaria na cache um valor que o banco não tem —
	// e como a entrada por user id não expira, esse valor errado ficaria lá
	// para sempre.
	if err := uc.users.UpdateUser(ctx, req.UserID, upd); err != nil {
		if errors.Is(err, ErrDuplicateToken) {
			return ErrDuplicateToken
		}
		if errors.Is(err, domain.ErrNoFieldsToUpdate) {
			// F206, decisão 48=a do canal: `token:""` significa CAMPO NÃO
			// INFORMADO, e um pedido sem nenhum campo útil é inválido — não
			// avaria nossa.
			//
			// Medido em campo a 2026-08-22: `{"token":""}` e `{}` devolviam
			// ambos 500 "internal server error" para algo determinístico, que
			// nunca muda de resposta por mais que o cliente repita. Mesma
			// família da F182 e da F204.
			//
			// Sem taxonomia o erro subia cru e o `RespondJSON` caía no ramo
			// genérico; com ela, o status vem da categoria e o cliente recebe
			// um código legível por máquina em vez de "internal server error".
			return apperr.New(noFieldsToUpdateCode, apperr.CategoryValidation,
				"request has no field to update", false, err)
		}
		uc.logger.Error(ctx, "Failed to update user", "error", err)
		return fmt.Errorf("database error: %w", err)
	}

	// Update S3Manager if needed
	if req.S3Config != nil {
		if req.S3Config.Enabled {
			s3Config := &storage.S3Config{
				Enabled:       req.S3Config.Enabled,
				Endpoint:      req.S3Config.Endpoint,
				Region:        req.S3Config.Region,
				Bucket:        req.S3Config.Bucket,
				AccessKey:     req.S3Config.AccessKey,
				SecretKey:     req.S3Config.SecretKey,
				PathStyle:     req.S3Config.PathStyle,
				PublicURL:     req.S3Config.PublicURL,
				MediaDelivery: req.S3Config.MediaDelivery,
				RetentionDays: req.S3Config.RetentionDays,
			}
			_ = storage.GetS3Manager().InitializeS3Client(req.UserID, s3Config)
		} else {
			storage.GetS3Manager().RemoveClient(req.UserID)
		}
	}

	// Depois da escrita, e só depois dela: o processo lê estes valores da
	// cache, não do banco. Sem isto, a edição entra no banco e fica invisível
	// até o processo reiniciar (F200), e o token substituído continua a
	// autenticar (F201).
	uc.republisher.RepublishUser(ctx, req.UserID)

	return nil
}
