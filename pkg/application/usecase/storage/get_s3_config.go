package storage

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// s3LoadFailedMsg is the historical text of the read 500
// (`41bc8e2^:handlers.go:6349`), shared with the connection test, which
// answers the same failure with the same message.
const s3LoadFailedMsg = "failed to get S3 configuration"

// s3RetrievedMsg is the success record of the read. It does NOT go to the
// body: the response of GET /s3/config is the configuration itself.
const s3RetrievedMsg = "S3 configuration retrieved"

// GetS3ConfigUseCase reads the S3 configuration WITHOUT the secret.
type GetS3ConfigUseCase struct {
	sessions appport.SessionGuard
	store    appport.S3ConfigStore
	logger   appport.Logger
}

// NewGetS3ConfigUseCase cria uma nova instância do usecase.
func NewGetS3ConfigUseCase(sg appport.SessionGuard, store appport.S3ConfigStore, l appport.Logger) *GetS3ConfigUseCase {
	return &GetS3ConfigUseCase{sessions: sg, store: store, logger: l}
}

// Execute returns the configuration with the access key masked and the secret
// absent.
//
// The two properties are independent, and both are load-bearing. The secret is
// absent because the READ does not select it, so no reshaping of the response
// can leak it. The access key is masked here, unconditionally, because it does
// come back from the database — the historical handler did the same
// (`41bc8e2^:handlers.go:6357`). Neither value reaches the log.
func (uc *GetS3ConfigUseCase) Execute(ctx context.Context, txtID string) (*domain.S3ConfigView, error) {
	if err := uc.sessions.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Warn(ctx, "no noise session", "txtID", txtID, "error", err)
		return nil, err
	}

	cfg, err := uc.store.LoadS3ConfigWithoutSecret(ctx, txtID)
	if err != nil {
		uc.logger.Error(ctx, s3LoadFailedMsg, "txtID", txtID, "error", err)
		return nil, fmt.Errorf("%s: %w", s3LoadFailedMsg, err)
	}
	if cfg == nil {
		// No user row: report the cleared configuration rather than an error,
		// so a caller can tell "nothing configured" from "read failed".
		cfg = &appport.S3ConfigRecord{
			PathStyle:     true,
			MediaDelivery: domain.DefaultMediaDelivery,
		}
	}

	view := &domain.S3ConfigView{
		Enabled:       cfg.Enabled,
		Endpoint:      cfg.Endpoint,
		Region:        cfg.Region,
		Bucket:        cfg.Bucket,
		AccessKey:     domain.MaskedS3AccessKey,
		PathStyle:     cfg.PathStyle,
		PublicURL:     cfg.PublicURL,
		MediaDelivery: cfg.MediaDelivery,
		RetentionDays: cfg.RetentionDays,
	}

	uc.logger.Info(ctx, s3RetrievedMsg, "txtID", txtID,
		"enabled", view.Enabled, "bucket", view.Bucket)
	return view, nil
}
