package storage

import (
	"context"
	"fmt"
	"time"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
	"wa-api/pkg/domain/apperr"
)

const (
	s3TestSuccessDetails  = "S3 connection test successful"
	s3NotEnabledMsg       = "S3 is not enabled for this user"
	s3NotEnabledCode      = "s3_not_enabled"
	s3DecryptFailedMsg    = "failed to decrypt the stored S3 secret"
	s3TestFailedFmt       = "S3 connection test failed: %v"
	s3TestConnectionLabel = "S3 connection test"

	// s3UpstreamRejectedCode is the SAME string value as
	// errmap.CodeUpstreamRejected ("upstream_rejected", pkg/infra/noise/
	// errmap/iqerror.go) — it names the same concept, "a well formed,
	// authorized request that the upstream refused" (apperr.
	// CategoryUpstreamRejected), just for a different upstream (S3, not
	// WhatsApp). It is declared separately, not imported: an application
	// use case importing pkg/infra/noise would invert the Clean
	// Architecture dependency direction (ADR-001).
	s3UpstreamRejectedCode = "upstream_rejected"
)

// s3TestTimeout is the ceiling of the single network round trip this use case
// makes, from the historical handler (`41bc8e2^:handlers.go:6443`).
const s3TestTimeout = 10 * time.Second

// TestS3ConnectionUseCase reaches the configured bucket with the STORED
// credential.
type TestS3ConnectionUseCase struct {
	sessions appport.SessionGuard
	store    appport.S3ConfigStore
	cipher   appport.S3SecretCipher
	clients  appport.S3ClientManager
	logger   appport.Logger
}

// NewTestS3ConnectionUseCase cria uma nova instância do usecase.
func NewTestS3ConnectionUseCase(
	sg appport.SessionGuard,
	store appport.S3ConfigStore,
	cipher appport.S3SecretCipher,
	clients appport.S3ClientManager,
	l appport.Logger,
) *TestS3ConnectionUseCase {
	return &TestS3ConnectionUseCase{sessions: sg, store: store, cipher: cipher, clients: clients, logger: l}
}

// Execute tests what is STORED, not what the caller sends.
//
// The request body is ignored on purpose: the historical handler read the
// configuration from the database (`41bc8e2^:handlers.go:6383`), and that is
// the only reading that answers the question the route is asked — "does the
// configuration I saved work?". Taking credentials from the body would test a
// credential that is not the one media upload will use.
//
// A stored secret that is not enveloped fails the test (ADR-0009). It is NOT
// used as a plaintext credential: doing so would silently keep a legacy
// plaintext row working, and the row would never be reconfigured.
func (uc *TestS3ConnectionUseCase) Execute(ctx context.Context, txtID string) (*domain.S3TestResult, error) {
	if err := uc.sessions.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Warn(ctx, "no noise session", "txtID", txtID, "error", err)
		return nil, err
	}

	cfg, err := uc.store.LoadS3Config(ctx, txtID)
	if err != nil {
		uc.logger.Error(ctx, s3LoadFailedMsg, "txtID", txtID, "error", err)
		return nil, fmt.Errorf("%s: %w", s3LoadFailedMsg, err)
	}
	if cfg == nil || !cfg.Enabled {
		uc.logger.Warn(ctx, s3NotEnabledMsg, "txtID", txtID)
		return nil, apperr.New(s3NotEnabledCode, apperr.CategoryValidation, s3NotEnabledMsg, false, nil)
	}

	plainSecret, err := uc.cipher.DecryptS3Secret(cfg.SecretKey)
	if err != nil {
		// The error carries no fragment of the stored value — not even its
		// prefix: what distinguishes the two failures is the cause chain.
		uc.logger.Error(ctx, s3DecryptFailedMsg, "txtID", txtID, "error", err)
		return nil, fmt.Errorf("%s: %w", s3DecryptFailedMsg, err)
	}

	live := *cfg
	live.SecretKey = plainSecret
	if err := uc.clients.InitializeS3Client(txtID, live); err != nil {
		uc.logger.Error(ctx, "failed to initialize S3 client", "txtID", txtID, "error", err)
		return nil, fmt.Errorf(s3InitClientFailedFmt, err)
	}

	testCtx, cancel := context.WithTimeout(ctx, s3TestTimeout)
	defer cancel()

	if err := uc.clients.TestConnection(testCtx, txtID); err != nil {
		uc.logger.Error(ctx, s3TestConnectionLabel+" failed", "txtID", txtID, "error", err)

		// F276: a refusal from the S3 endpoint (bad credentials, wrong
		// bucket, network reachable but access denied) is a CLIENT-side
		// problem with the stored configuration, not a server failure —
		// the same class the F204/errmap precedent already covers for
		// WhatsApp IQ refusals. Untyped, this fell through RespondJSON's
		// generic 500 branch: the one response this diagnostic ROUTE exists
		// to give ("is my configuration good?") was the least useful one
		// possible.
		//
		// The message carries the upstream's own text — that IS the
		// answer the caller asked for — but never the stored secret: only
		// the AWS SDK's own error text (status, access key ID, request ID)
		// reaches here, never cfg.SecretKey/plainSecret.
		return nil, apperr.New(
			s3UpstreamRejectedCode,
			apperr.CategoryUpstreamRejected,
			fmt.Sprintf(s3TestFailedFmt, err),
			false,
			err,
		)
	}

	uc.logger.Info(ctx, s3TestSuccessDetails, "txtID", txtID, "bucket", cfg.Bucket, "region", cfg.Region)
	return &domain.S3TestResult{
		Connected: true,
		Details:   s3TestSuccessDetails,
		Bucket:    cfg.Bucket,
		Region:    cfg.Region,
	}, nil
}
