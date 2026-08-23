package message

import (
	"context"
	"net/http"
	"strings"

	"github.com/vincent-petithory/dataurl"

	"wa-api/pkg/domain/apperr"

	appport "wa-api/pkg/application/contracts"
	"wa-api/pkg/domain"
)

// carouselCardImageMaxBytes mirrors buttonsHeaderImageMaxBytes: same origin,
// same limit, same reason (parity with the 10 MB ceiling the project has
// always used for header images — see send_buttons.go).
const carouselCardImageMaxBytes int64 = 10 * 1024 * 1024

// SendCarouselUseCase sends a carousel interactive message: normalises the
// buttons of every card, obtains each card's image bytes (URL or data URI,
// same dual-source as SendButtonsUseCase.headerImageBytes), and delegates to
// port.InteractiveMessenger.SendCarousel.
//
// Only HSCROLL_CARDS is accepted. ALBUM_IMAGE exists in the domain and in
// the adapter (it is protocol truth), but does NOT render on current WhatsApp
// clients (HOUSEKEEP F211) and is therefore excluded from the public surface.
type SendCarouselUseCase struct {
	messages appport.InteractiveMessenger
	jids     appport.JIDResolver
	fetcher  appport.MediaFetcher
	logger   appport.Logger
}

// NewSendCarouselUseCase creates the use case.
func NewSendCarouselUseCase(im appport.InteractiveMessenger, jr appport.JIDResolver, mf appport.MediaFetcher, l appport.Logger) *SendCarouselUseCase {
	return &SendCarouselUseCase{
		messages: im,
		jids:     jr,
		fetcher:  mf,
		logger:   l,
	}
}

// Execute validates, normalises and sends the carousel.
func (uc *SendCarouselUseCase) Execute(ctx context.Context, txtID string, req domain.SendCarouselRequest) (*domain.SendCarouselResult, error) {
	body := strings.TrimSpace(req.Body)
	if req.Phone == "" || body == "" || len(req.Cards) == 0 {
		return nil, apperr.New("missing_carousel_fields", apperr.CategoryValidation,
			"missing Phone, Body or Cards", false, nil)
	}

	if err := uc.messages.EnsureSession(ctx, txtID); err != nil {
		uc.logger.Warn(ctx, "no wanoise session", "txtID", txtID, "error", err)
		return nil, err
	}

	recipient, err := uc.jids.ResolveJID(ctx, req.Phone)
	if err != nil {
		uc.logger.Warn(ctx, "invalid phone in send carousel payload", "txtID", txtID, "error", err)
		return nil, apperr.New("invalid_phone", apperr.CategoryValidation, "could not parse Phone", false, nil)
	}

	cards := make([]domain.CarouselCard, 0, len(req.Cards))
	for i, c := range req.Cards {
		cardBody := strings.TrimSpace(c.Body)
		if cardBody == "" {
			uc.logger.Warn(ctx, "carousel card dropped: empty body", "txtID", txtID, "cardIndex", i)
			continue
		}

		buttons, dropped := normalizeInteractiveButtons(c.Buttons)
		for _, d := range dropped {
			uc.logger.Warn(ctx, "carousel card button dropped: unknown type",
				"txtID", txtID,
				"clientMsgID", req.ID,
				"cardIndex", i,
				"receivedType", d.ReceivedType,
				"title", d.Title,
				"reason", buttonDropReasonUnknownType,
				"acceptedTypes", acceptedButtonTypes())
		}
		if len(buttons) == 0 {
			uc.logger.Warn(ctx, "carousel card dropped: no valid buttons", "txtID", txtID, "cardIndex", i)
			continue
		}

		image := uc.cardImageBytes(ctx, txtID, c.Image, i)

		card := domain.CarouselCard{
			Title:   strings.TrimSpace(c.Title),
			Body:    cardBody,
			Footer:  strings.TrimSpace(c.Footer),
			Buttons: buttons,
			Image:   image,
		}
		if len(image) > 0 {
			card.ImageMimeType = http.DetectContentType(image)
		}
		cards = append(cards, card)
	}

	if len(cards) == 0 {
		return nil, apperr.New("no_valid_cards", apperr.CategoryValidation,
			"no valid cards after normalisation, accepted button types: "+acceptedButtonTypes(), false, nil)
	}

	payload := domain.CarouselPayload{
		Body:     body,
		Footer:   strings.TrimSpace(req.Footer),
		CardType: domain.CarouselHScrollCards,
		Cards:    cards,
	}

	sent, err := uc.messages.SendCarousel(ctx, txtID, recipient, payload, req.ReplyTo, req.ID)
	if err != nil {
		uc.logger.Error(ctx, "failed to send carousel message", "txtID", txtID, "error", err)
		return nil, err
	}

	result := &domain.SendCarouselResult{
		MessageID: sent.ID,
		Timestamp: sent.Timestamp.Unix(),
		Status:    domain.StatusSent,
	}

	uc.logger.Info(ctx, "carousel sent", "msgID", result.MessageID, "cards", len(cards))
	return result, nil
}

// cardImageBytes obtains the image bytes for one card, or returns nil.
// Same silent-discard discipline as SendButtonsUseCase.headerImageBytes.
func (uc *SendCarouselUseCase) cardImageBytes(ctx context.Context, txtID, image string, cardIndex int) []byte {
	if image == "" {
		return nil
	}

	var data []byte

	switch {
	case isDataURIImage(image):
		decoded, err := dataurl.DecodeString(image)
		if err != nil {
			uc.logger.Warn(ctx, "carousel card image dropped: data uri did not decode",
				"txtID", txtID, "cardIndex", cardIndex, "error", err)
			return nil
		}
		data = decoded.Data

	case isHTTPImageURL(image):
		fetched, _, err := uc.fetcher.FetchBytes(ctx, image, carouselCardImageMaxBytes)
		if err != nil {
			uc.logger.Warn(ctx, "carousel card image dropped: fetch failed",
				"txtID", txtID, "cardIndex", cardIndex, "error", err)
			return nil
		}
		data = fetched

	default:
		uc.logger.Warn(ctx, "carousel card image dropped: unsupported source",
			"txtID", txtID, "cardIndex", cardIndex)
		return nil
	}

	if len(data) == 0 {
		uc.logger.Warn(ctx, "carousel card image dropped: empty body",
			"txtID", txtID, "cardIndex", cardIndex)
		return nil
	}
	if int64(len(data)) > carouselCardImageMaxBytes {
		uc.logger.Warn(ctx, "carousel card image dropped: exceeds size limit",
			"txtID", txtID, "cardIndex", cardIndex, "bytes", len(data), "limit", carouselCardImageMaxBytes)
		return nil
	}

	return data
}
