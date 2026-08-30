package user

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"wa-api/pkg/domain/apperr"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	dbpkg "wa-api/pkg/infra/db"
	"wa-api/pkg/infra/storage"
)

// Error codes and messages of this use case, as named constants: the tests
// that lock the contract assert the SAME strings production returns
// (ADR-0004).
const (
	hmacKeyTooShortCode  = "hmac_key_too_short"
	hmacKeyTooShortMsg   = "HMAC key must be at least 32 characters long"
	hmacEncryptFailedMsg = "failed to encrypt HMAC key"

	s3SecretEncryptFailedMsg = "failed to encrypt S3 secret key"

	invalidEventTypeCode   = "invalid_event_type"
	invalidEventTypeMsgFmt = "invalid event type: %s"

	// noFieldsToUpdateCode identifica um PUT que não traz campo algum a mudar.
	// Constante nomeada, e não literal repetido, porque o teste de contrato o
	// afirma e o cliente o lê (ADR-0004).
	noFieldsToUpdateCode = "no_fields_to_update"

	// invalidEngineCode marca a criação recusada por engine ausente, nula,
	// vazia ou fora de {noise, headless} (itens 4-5). Mensagem em
	// pt-BR, convenção do projeto para o campo error.message do envelope.
	invalidEngineCode = "invalid_engine"
	invalidEngineMsg  = "engine é obrigatório e deve ser \"noise\" ou \"headless\""

	// engineImmutableCode marca uma tentativa de mudar o engine de um
	// usuário já existente (itens 8, 61) — ver EditUserUseCase.Execute.
	engineImmutableCode = "engine_immutable"
	engineImmutableMsg  = "engine não pode ser alterado após a criação da conta"

	// engineHeadlessUnavailableCode identifica um pedido de sessão em
	// domain.EngineHeadless num servidor sem o Chrome do headless
	// configurado. Recusar aqui é a mesma regra de "sem fallback silencioso"
	// da decisão 94 (pkg/bootstrap/engine_routing.go): pedir um engine que o
	// processo não pode servir tem de FALHAR, nunca cair para o socket em
	// silêncio.
	engineHeadlessUnavailableCode = "engine_headless_unavailable"
	engineHeadlessUnavailableMsg  = "headless engine is not configured on this server"
)

// AddUserUseCase adiciona um novo usuário
type AddUserUseCase struct {
	users     appport.UserRepository
	encryptor appport.HmacKeyEncryptor
	s3Cipher  appport.S3SecretCipher
	logger    appport.Logger

	// headlessAvailable diz se este processo tem o Chrome do headless
	// configurado (s.Headless.ChromePath != "" no bootstrap). Substitui a
	// checagem estática que a decisão 94 fazia no arranque: agora é
	// checagem por REQUISIÇÃO, porque a escolha do engine também passou a
	// ser por requisição.
	headlessAvailable bool
}

// NewAddUserUseCase cria uma nova instância.
//
// The encryptor and s3Cipher are dependencies and not package-level helpers
// because the AES key lives in the process configuration
// (appCtx.GlobalEncryptionKey), which the application layer must not reach
// for. Each port matches a column writer: encryptor → users.hmac_key
// (F158), s3Cipher → users.s3_secret_key (F163). Separate ports because
// the two columns have different stored types (BYTEA vs TEXT envelope).
//
// headlessAvailable is whether this server has the headless engine
// configured at all — see the field comment on AddUserUseCase.
func NewAddUserUseCase(users appport.UserRepository, encryptor appport.HmacKeyEncryptor, s3Cipher appport.S3SecretCipher, logger appport.Logger, headlessAvailable bool) *AddUserUseCase {
	return &AddUserUseCase{users: users, encryptor: encryptor, s3Cipher: s3Cipher, logger: logger, headlessAvailable: headlessAvailable}
}

// Execute adiciona um novo usuário
func (uc *AddUserUseCase) Execute(ctx context.Context, req domain.AddUserInput) (*domain.UserAccount, error) {
	// Validate required fields
	if req.Name == "" || req.Token == "" {
		return nil, apperr.New("missing_name_or_token", apperr.CategoryValidation, "name and token are required", false, nil)
	}

	// Engine é OBRIGATÓRIO na criação (itens 4-5 do prompt arquitetural):
	// ausente/nulo/vazio/inválido é 400 invalid_engine, nunca um default
	// silencioso. domain.ParseEngine já rejeita "" e "legacy_unknown" — o
	// segundo é valor interno de leitura, nunca escolha de criação.
	engine, err := domain.ParseEngine(req.Engine)
	if err != nil {
		return nil, apperr.New(invalidEngineCode, apperr.CategoryValidation, invalidEngineMsg, false, err)
	}
	if engine == domain.EngineHeadless && !uc.headlessAvailable {
		return nil, apperr.New(engineHeadlessUnavailableCode, apperr.CategoryValidation,
			engineHeadlessUnavailableMsg, false, nil)
	}

	// Set defaults
	if req.ProxyConfig == nil {
		req.ProxyConfig = &domain.ProxyConfig{}
	}
	if req.S3Config == nil {
		req.S3Config = &domain.S3Config{}
	}

	webhookUseProxy := true
	if req.ProxyConfig.WebhookUseProxy != nil {
		webhookUseProxy = *req.ProxyConfig.WebhookUseProxy
	}

	// Encrypt the HMAC key if provided.
	//
	// The ORDER is the contract: validate, then encrypt, then write. A failure
	// to encrypt returns BEFORE CreateUser, so the user is not created with an
	// empty key nor with the key in plaintext — the column is read back
	// through auth.DecryptHMACKey to sign every per-user webhook, and
	// plaintext there is not valid AES-GCM.
	var encryptedHmacKey []byte
	if req.HmacKey != "" {
		if len(req.HmacKey) < domain.MinHmacKeyLength {
			return nil, apperr.New(hmacKeyTooShortCode, apperr.CategoryValidation, hmacKeyTooShortMsg, false, nil)
		}
		// The plaintext key never reaches a log or an error: only its length does.
		encrypted, err := uc.encryptor.EncryptHmacKey(req.HmacKey)
		if err != nil {
			uc.logger.Error(ctx, hmacEncryptFailedMsg, "keyLength", len(req.HmacKey), "error", err)
			return nil, fmt.Errorf("%s: %w", hmacEncryptFailedMsg, err)
		}
		encryptedHmacKey = encrypted
	}

	// Encrypt the S3 secret key if provided (F163, ADR-0009).
	//
	// Same ORDER contract as the HMAC key above: encrypt before write. A
	// failure returns BEFORE CreateUser, so the column never holds plaintext.
	// The in-memory S3 client (below) receives the PLAINTEXT — the AWS SDK
	// signs with it, and the envelope would produce a client that fails every
	// request.
	var s3SecretEnvelope string
	if req.S3Config != nil && req.S3Config.SecretKey != "" {
		envelope, err := uc.s3Cipher.EncryptS3Secret(req.S3Config.SecretKey)
		if err != nil {
			uc.logger.Error(ctx, s3SecretEncryptFailedMsg, "error", err)
			return nil, fmt.Errorf("%s: %w", s3SecretEncryptFailedMsg, err)
		}
		s3SecretEnvelope = envelope
	}

	// Validate events
	if req.Events != "" {
		eventList := strings.Split(req.Events, ",")
		for _, event := range eventList {
			event = strings.TrimSpace(event)
			if event == "" {
				continue
			}
			if !isValidEvent(event) {
				return nil, apperr.New(invalidEventTypeCode, apperr.CategoryValidation,
					fmt.Sprintf(invalidEventTypeMsgFmt, event), false, nil)
			}
		}
	}

	// Generate ID
	id, err := dbpkg.GenerateRandomID()
	if err != nil {
		uc.logger.Error(ctx, "Failed to generate ID", "error", err)
		return nil, fmt.Errorf("failed to generate user ID: %w", err)
	}

	s3ForRecord := *req.S3Config
	if s3SecretEnvelope != "" {
		s3ForRecord.SecretKey = s3SecretEnvelope
	}
	created, err := uc.users.CreateUser(ctx, domain.UserRecord{
		ID:              id,
		Name:            req.Name,
		Token:           req.Token,
		Webhook:         req.Webhook,
		Expiration:      req.Expiration,
		Events:          req.Events,
		ProxyURL:        req.ProxyConfig.ProxyURL,
		WebhookUseProxy: webhookUseProxy,
		S3:              s3ForRecord,
		HmacKey:         encryptedHmacKey,
		History:         req.History,
		Engine:          engine,
	})
	if err != nil {
		if errors.Is(err, ErrDuplicateToken) {
			return nil, ErrDuplicateToken
		}
		uc.logger.Error(ctx, "Failed to insert user", "error", err)
		return nil, fmt.Errorf("database error: %w", err)
	}
	if !created {
		return nil, ErrDuplicateToken
	}

	// Initialize S3 if enabled
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
		_ = storage.GetS3Manager().InitializeS3Client(id, s3Config)
	}

	// Build the result. It is a domain value, not a payload: the key names
	// the caller will see are decided by dtoadmin's presenter.
	return &domain.UserAccount{
		ID:             id,
		Name:           req.Name,
		Token:          req.Token,
		Webhook:        req.Webhook,
		Expiration:     int64(req.Expiration),
		Events:         req.Events,
		HmacConfigured: req.HmacKey != "",
		Engine:         engine.String(),
		Proxy: domain.UserProxySettings{
			Enabled:         req.ProxyConfig.ProxyURL != "",
			URL:             req.ProxyConfig.ProxyURL,
			WebhookUseProxy: webhookUseProxy,
		},
		S3: domain.UserS3Settings{
			Enabled:             req.S3Config.Enabled,
			Endpoint:            req.S3Config.Endpoint,
			Region:              req.S3Config.Region,
			Bucket:              req.S3Config.Bucket,
			PathStyle:           req.S3Config.PathStyle,
			PublicURL:           req.S3Config.PublicURL,
			MediaDelivery:       req.S3Config.MediaDelivery,
			RetentionDays:       req.S3Config.RetentionDays,
			AccessKeyConfigured: req.S3Config.AccessKey != "",
		},
	}, nil
}

// isValidEvent reports whether the event name is one of
// domain.SupportedEventTypes.
//
// Rejecting an unknown event is a NEW public contract, decided deliberately
// (HOUSEKEEP F159) — it is not the recovery of an older behaviour. There is
// no older behaviour to recover: this function used to return true for every
// input, so the 400 above was dead code, and the pre-migration handler did
// not validate events at all. Callers that today send a misspelled event and
// get 200 will start getting 400.
//
// The alternative — dropping the unknown entry and carrying on, which the
// sibling UpdateWebhook route does — was rejected: a rejection is visible to
// the integrator at the moment of the call and is fixable there, while a
// silent drop only surfaces later, to the operator, as an event that never
// arrives.
//
// The validator is domain.IsValidEventType and not the identical list in
// pkg/infra/constants (HOUSEKEEP F168) because this is the application layer:
// importing infra from here would invert the dependency direction, and
// pkg/domain depends on nobody.
func isValidEvent(event string) bool {
	return domain.IsValidEventType(event)
}
