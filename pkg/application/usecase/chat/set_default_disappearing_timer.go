package chat

import (
	"context"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain/apperr"
)

// SetDefaultDisappearingTimerUseCase sets the account-wide default
// disappearing message timer for new conversations.
type SetDefaultDisappearingTimerUseCase struct {
	setter appport.DefaultDisappearingTimerSetter
	logger appport.Logger
}

// NewSetDefaultDisappearingTimerUseCase creates a new instance.
func NewSetDefaultDisappearingTimerUseCase(s appport.DefaultDisappearingTimerSetter, l appport.Logger) *SetDefaultDisappearingTimerUseCase {
	return &SetDefaultDisappearingTimerUseCase{setter: s, logger: l}
}

// Execute sets the default disappearing timer.
// duration is validated against the discrete set the WhatsApp server
// accepts: off, 24h, 7d, 90d.
func (uc *SetDefaultDisappearingTimerUseCase) Execute(ctx context.Context, txtID, duration string) error {
	if err := uc.setter.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Warn(ctx, "no noise session", "txtID", txtID, "error", err)
		return err
	}

	d, ok := parseDisappearingDuration(duration)
	if !ok {
		uc.logger.Warn(ctx, "invalid default disappearing timer duration", "txtID", txtID, "duration", duration)
		return apperr.New("invalid_duration", apperr.CategoryValidation,
			"duration must be one of: 0, 24h, 7d, 90d", false, nil)
	}

	if err := uc.setter.SetDefaultDisappearingTimer(ctx, txtID, d); err != nil {
		uc.logger.Error(ctx, "failed to set default disappearing timer", "txtID", txtID, "duration", duration, "error", err)
		return err
	}

	return nil
}
