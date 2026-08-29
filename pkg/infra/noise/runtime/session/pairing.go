package session

import (
	"context"
	"errors"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain/apperr"

	"wa-api/internal/noise"
)

// Pair inicia o fluxo de QR e conecta o transporte (a conexão é o que faz o
// noise emitir os códigos). O canal devolvido é fechado quando o canal
// do SDK fecha.
func (s *noiseSession) Pair(ctx context.Context) (<-chan appport.PairingEvent, error) {
	if s.HasCredentials() {
		return nil, apperr.New(codeSessionAlreadyPaired, apperr.CategoryValidation, "session already has credentials", false, nil)
	}

	qrChan, err := s.client.GetQRChannel(ctx)
	if err != nil {
		if errors.Is(err, noise.ErrQRStoreContainsID) {
			return nil, apperr.New(codeSessionAlreadyPaired, apperr.CategoryValidation, "session already has credentials", false, err)
		}
		return nil, apperr.New("qr_channel_failed", apperr.CategoryInternal, "failed to get QR channel", true, err)
	}

	if err := s.client.Connect(); err != nil {
		return nil, apperr.New("qr_connect_failed", apperr.CategoryInternal, "failed to connect client for QR pairing", true, err)
	}

	out := make(chan appport.PairingEvent)
	go func() {
		defer close(out)
		for item := range qrChan {
			evt, ok := s.translatePairingItem(item)
			if !ok {
				continue
			}
			select {
			case out <- evt:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

func (s *noiseSession) translatePairingItem(item noise.QRChannelItem) (appport.PairingEvent, bool) {
	switch item.Event {
	case noise.QRChannelEventCode:
		return appport.PairingEvent{
			Kind:    appport.PairingEventKindQR,
			Code:    item.Code,
			Timeout: item.Timeout,
		}, true
	case "timeout":
		return appport.PairingEvent{Kind: appport.PairingEventKindTimeout}, true
	case "success":
		jid, _ := s.JID()
		return appport.PairingEvent{Kind: appport.PairingEventKindSuccess, JID: jid}, true
	default:
		return appport.PairingEvent{}, false
	}
}

// codeSessionAlreadyPaired é devolvido por dois caminhos distintos de
// Pair (checagem local de credenciais e ErrQRStoreContainsID do SDK) que
// o caller precisa tratar da mesma forma.
const codeSessionAlreadyPaired = "session_already_paired"
