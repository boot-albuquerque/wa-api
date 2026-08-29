package storage

import (
	"context"
	"fmt"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

const (
	s3DeletedDetails  = "S3 configuration deleted successfully"
	s3DeleteFailedMsg = "failed to delete S3 configuration"
)

// DeleteS3ConfigUseCase revokes the per-user S3 credential.
type DeleteS3ConfigUseCase struct {
	sessions appport.SessionGuard
	store    appport.S3ConfigStore
	clients  appport.S3ClientManager
	cache    appport.UserInfoS3Cache
	logger   appport.Logger
}

// NewDeleteS3ConfigUseCase cria uma nova instância do usecase.
func NewDeleteS3ConfigUseCase(
	sg appport.SessionGuard,
	store appport.S3ConfigStore,
	clients appport.S3ClientManager,
	cache appport.UserInfoS3Cache,
	l appport.Logger,
) *DeleteS3ConfigUseCase {
	return &DeleteS3ConfigUseCase{sessions: sg, store: store, clients: clients, cache: cache, logger: l}
}

// Execute clears the database AND drops the in-memory client — in this order,
// and both parts are mandatory.
//
// Clearing only the database revokes nothing: S3Manager keeps the client it
// built from the previous credential (pkg/infra/storage/s3.go:140), and every
// media upload after the DELETE would still reach the third-party bucket with
// the credential the operator believes they cut. The response is 200 either
// way, so only the state of the registry tells the two apart (HOUSEKEEP F157).
//
// Dropping the client BEFORE the write would be the mirrored defect: a failed
// UPDATE would leave the revocation half-done, and the next lazy init
// (EnsureClientFromDB) would rebuild the client from the row that never
// changed. So the registry is only touched after the write succeeded.
func (uc *DeleteS3ConfigUseCase) Execute(ctx context.Context, txtID string) (*domain.S3ConfigResult, error) {
	if err := uc.sessions.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Warn(ctx, "no noise session", "txtID", txtID, "error", err)
		return nil, err
	}

	if err := uc.store.DeleteS3Config(ctx, txtID); err != nil {
		uc.logger.Error(ctx, s3DeleteFailedMsg, "txtID", txtID, "error", err)
		return nil, fmt.Errorf("%s: %w", s3DeleteFailedMsg, err)
	}

	uc.clients.RemoveClient(txtID)
	uc.cache.SetS3Config(txtID, false, domain.DefaultMediaDelivery)

	uc.logger.Info(ctx, s3DeletedDetails, "txtID", txtID)
	return &domain.S3ConfigResult{Details: s3DeletedDetails, Enabled: false}, nil
}
