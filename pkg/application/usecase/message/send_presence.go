package message

import (
	"context"
	"fmt"
	"wa-api/pkg/domain/apperr"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// Error codes and messages of this use case, as named constants: the tests
// that lock the contract assert the SAME strings production returns
// (ADR-0004).
//
// missing_presence_type and invalid_presence_type are distinct on purpose: an
// absent `type` and a misspelled `type` are errors with DIFFERENT
// corrections, and a client branching on error.code could not tell them apart
// while both answered invalid_presence_type.
const (
	missingPresenceTypeCode = "missing_presence_type"
	missingPresenceTypeMsg  = "missing type in payload"

	invalidPresenceTypeCode   = "invalid_presence_type"
	invalidPresenceTypeMsgFmt = "invalid presence type %q. Allowed values: 'available', 'unavailable'"
)

// SendPresenceUseCase sets global presence status
type SendPresenceUseCase struct {
	presence appport.PresenceAnnouncer
	logger   appport.Logger
}

// NewSendPresenceUseCase creates a new instance
func NewSendPresenceUseCase(pc appport.PresenceAnnouncer, logger appport.Logger) *SendPresenceUseCase {
	return &SendPresenceUseCase{presence: pc, logger: logger}
}

// Execute sets presence status
func (uc *SendPresenceUseCase) Execute(ctx context.Context, userID string, req domain.SendPresenceRequest) error {
	if err := uc.presence.EnsureSession(ctx, userID); err != nil {
		uc.logger.Warn(ctx, "no wanoise session", "error", err, "user_id", userID)
		return err
	}

	// Absent, null and "" are indistinguishable once the payload is decoded
	// into a Go string, so the three collapse into the same answer: the field
	// is missing. Only a NON-EMPTY value outside the set is "invalid".
	if req.Type == "" {
		return apperr.New(missingPresenceTypeCode, apperr.CategoryValidation, missingPresenceTypeMsg, false, nil)
	}

	var presence domain.PresenceType
	switch req.Type {
	case string(domain.PresenceAvailable):
		presence = domain.PresenceAvailable
	case string(domain.PresenceUnavailable):
		presence = domain.PresenceUnavailable
	default:
		return apperr.New(invalidPresenceTypeCode, apperr.CategoryValidation,
			fmt.Sprintf(invalidPresenceTypeMsgFmt, req.Type), false, nil)
	}

	uc.logger.Info(ctx, "Setting presence", "presence", req.Type, "user_id", userID)

	if err := uc.presence.SendPresence(ctx, userID, presence); err != nil {
		uc.logger.Error(ctx, "Failed to send presence", "error", err, "user_id", userID)
		return fmt.Errorf("failure sending presence to Whatsapp servers")
	}

	return nil
}
