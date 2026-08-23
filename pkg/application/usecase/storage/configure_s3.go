package storage

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
	"wa-api/pkg/infra/egress"
)

// Result and error messages of the S3 write path, as constants: the test that
// locks the contract asserts the SAME strings production returns (ADR-0004).
// "saved successfully" is the historical text (`41bc8e2^:handlers.go:6313`).
const (
	s3ConfiguredDetails   = "S3 configuration saved successfully"
	s3MediaDeliveryMsg    = "media_delivery must be 'base64', 's3', or 'both'"
	s3InvalidEndpointMsg  = "invalid S3 endpoint"
	s3EncryptFailedMsg    = "failed to encrypt the S3 secret"
	s3SaveFailedMsg       = "failed to save S3 configuration"
	s3InitClientFailedFmt = "failed to initialize S3 client: %v"
	s3MediaDeliveryCode   = "invalid_media_delivery"
	s3InvalidEndpointCode = "invalid_s3_endpoint"
)

// ConfigureS3UseCase writes the per-user S3 configuration.
type ConfigureS3UseCase struct {
	sessions appport.SessionGuard
	store    appport.S3ConfigStore
	cipher   appport.S3SecretCipher
	clients  appport.S3ClientManager
	cache    appport.UserInfoS3Cache
	logger   appport.Logger
}

// NewConfigureS3UseCase cria uma nova instância do usecase.
func NewConfigureS3UseCase(
	sg appport.SessionGuard,
	store appport.S3ConfigStore,
	cipher appport.S3SecretCipher,
	clients appport.S3ClientManager,
	cache appport.UserInfoS3Cache,
	l appport.Logger,
) *ConfigureS3UseCase {
	return &ConfigureS3UseCase{
		sessions: sg,
		store:    store,
		cipher:   cipher,
		clients:  clients,
		cache:    cache,
		logger:   l,
	}
}

// Execute validates, encrypts, writes, and only then registers the client.
//
// The ORDER is the contract, not an implementation detail:
//
//  1. validate — an invalid media_delivery or endpoint never reaches the
//     cipher, and nothing is written;
//  2. encrypt — a failure here must NOT write, or the TEXT column would hold
//     the credential in the clear, which is exactly what ADR-0009 diverges
//     from the historical handler to avoid;
//  3. write — the source of truth first;
//  4. client registry — a failure answers 500 with the row already saved,
//     which is the historical behaviour (`41bc8e2^:handlers.go:6288`): the
//     configuration is what the user asked to store, and the connection is
//     what /s3/test exists to check;
//  5. cache last, and only the two fields that readers consume.
//
// The plaintext secret never enters a log nor an error: what is logged about
// it is whether one was supplied.
func (uc *ConfigureS3UseCase) Execute(ctx context.Context, txtID string, req domain.S3ConfigRequest) (*domain.S3ConfigResult, error) {
	if err := uc.sessions.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Warn(ctx, "no wanoise session", "txtID", txtID, "error", err)
		return nil, err
	}

	if req.MediaDelivery != "" && !domain.IsValidMediaDelivery(req.MediaDelivery) {
		uc.logger.Warn(ctx, s3MediaDeliveryMsg, "txtID", txtID, "mediaDelivery", req.MediaDelivery)
		return nil, apperr.New(s3MediaDeliveryCode, apperr.CategoryValidation, s3MediaDeliveryMsg, false, nil)
	}

	// Endpoint is optional (empty means the default AWS S3 endpoint); when
	// set, it had no validation at all before this (sec/F24).
	if req.Endpoint != "" {
		if err := egress.ValidateOutboundURL(ctx, req.Endpoint); err != nil {
			uc.logger.Warn(ctx, s3InvalidEndpointMsg, "txtID", txtID, "error", err)
			return nil, apperr.New(s3InvalidEndpointCode, apperr.CategoryValidation,
				s3InvalidEndpointMsg, false, err)
		}
	}

	if req.MediaDelivery == "" {
		req.MediaDelivery = domain.DefaultMediaDelivery
	}

	envelope, err := uc.cipher.EncryptS3Secret(req.SecretKey)
	if err != nil {
		uc.logger.Error(ctx, s3EncryptFailedMsg, "txtID", txtID, "error", err)
		return nil, fmt.Errorf("%s: %w", s3EncryptFailedMsg, err)
	}

	stored := appport.S3ConfigRecord{
		Enabled:       req.Enabled,
		Endpoint:      req.Endpoint,
		Region:        req.Region,
		Bucket:        req.Bucket,
		AccessKey:     req.AccessKey,
		SecretKey:     envelope,
		PathStyle:     req.PathStyle,
		PublicURL:     req.PublicURL,
		MediaDelivery: req.MediaDelivery,
		RetentionDays: req.RetentionDays,
	}
	if err := uc.store.SaveS3Config(ctx, txtID, stored); err != nil {
		uc.logger.Error(ctx, s3SaveFailedMsg, "txtID", txtID, "error", err)
		return nil, fmt.Errorf("%s: %w", s3SaveFailedMsg, err)
	}

	// The client registry gets the PLAINTEXT secret — the AWS SDK signs with
	// it, and the envelope would produce a client that fails every request.
	if req.Enabled {
		live := stored
		live.SecretKey = req.SecretKey
		if err := uc.clients.InitializeS3Client(txtID, live); err != nil {
			uc.logger.Error(ctx, "failed to initialize S3 client", "txtID", txtID, "error", err)
			return nil, fmt.Errorf(s3InitClientFailedFmt, err)
		}
	} else {
		// Disabling REVOKES: the client built from the previous credential
		// would keep uploading media otherwise (`41bc8e2^:handlers.go:6292`).
		uc.clients.RemoveClient(txtID)
	}

	uc.cache.SetS3Config(txtID, req.Enabled, req.MediaDelivery)

	uc.logger.Info(ctx, s3ConfiguredDetails, "txtID", txtID,
		"enabled", req.Enabled, "mediaDelivery", req.MediaDelivery, "hasSecret", req.SecretKey != "")
	return &domain.S3ConfigResult{Details: s3ConfiguredDetails, Enabled: req.Enabled}, nil
}
