package handlers

import (
	"net/http"
	"time"

	"github.com/rs/zerolog/hlog"

	"wa-api/pkg/application/usecase/notification"
	"wa-api/pkg/domain"
	customhttp "wa-api/pkg/presentation/http"
)

// NewsletterHandlers groups the eleven newsletter operation handlers.
//
// WHY ONE HANDLER TYPE AND NOT ELEVEN. The eleven routes differ only in which
// NewsletterOp they carry: the decode, the session lookup, the error mapping
// and the response envelope are identical. Eleven copies of that shape are
// eleven places for it to drift — the same reasoning that made the use case a
// single dispatch table instead of eleven use cases.
type NewsletterHandlers struct {
	Create            *newsletterOpHandler
	Info              *newsletterOpHandler
	InfoInvite        *newsletterOpHandler
	Follow            *newsletterOpHandler
	Unfollow          *newsletterOpHandler
	Mute              *newsletterOpHandler
	Messages          *newsletterOpHandler
	Updates           *newsletterOpHandler
	MarkViewed        *newsletterOpHandler
	React             *newsletterOpHandler
	Subscribe         *newsletterOpHandler
	Demote            *newsletterOpHandler
	ChangeOwner       *newsletterOpHandler
	Delete            *newsletterOpHandler
	AdminInvite       *newsletterOpHandler
	AdminInviteAccept *newsletterOpHandler
	AdminInviteRevoke *newsletterOpHandler
}

// newsletterBody is the wire shape shared by the eleven routes. Each operation
// reads only the fields it needs; the use case is what enforces which of them
// are mandatory, so a missing field fails validation there rather than being
// silently accepted here.
type newsletterBody struct {
	JID    string `json:"jid"`
	Invite string `json:"invite"`

	Name        string `json:"name"`
	Description string `json:"description"`
	Picture     []byte `json:"picture"`

	Mute bool `json:"mute"`

	Count  int    `json:"count"`
	Before string `json:"before"`
	After  string `json:"after"`
	Since  string `json:"since"`

	ServerIDs []int  `json:"serverIDs"`
	ServerID  int    `json:"serverID"`
	Reaction  string `json:"reaction"`
	MessageID string `json:"messageID"`

	UserJID    string `json:"userJID"`
	ConfirmJID string `json:"confirmJID"`
}

type newsletterOpHandler struct {
	uc *notification.NewsletterOpsUseCase
	op notification.NewsletterOp
}

func newNewsletterOpHandler(uc *notification.NewsletterOpsUseCase, op notification.NewsletterOp) *newsletterOpHandler {
	return &newsletterOpHandler{uc: uc, op: op}
}

// NewNewsletterHandlers creates all newsletter operation handlers.
func NewNewsletterHandlers(uc *notification.NewsletterOpsUseCase) *NewsletterHandlers {
	return &NewsletterHandlers{
		Create:            newNewsletterOpHandler(uc, notification.NewsletterOpCreate),
		Info:              newNewsletterOpHandler(uc, notification.NewsletterOpInfo),
		InfoInvite:        newNewsletterOpHandler(uc, notification.NewsletterOpInfoInvite),
		Follow:            newNewsletterOpHandler(uc, notification.NewsletterOpFollow),
		Unfollow:          newNewsletterOpHandler(uc, notification.NewsletterOpUnfollow),
		Mute:              newNewsletterOpHandler(uc, notification.NewsletterOpMute),
		Messages:          newNewsletterOpHandler(uc, notification.NewsletterOpMessages),
		Updates:           newNewsletterOpHandler(uc, notification.NewsletterOpUpdates),
		MarkViewed:        newNewsletterOpHandler(uc, notification.NewsletterOpMarkViewed),
		React:             newNewsletterOpHandler(uc, notification.NewsletterOpReact),
		Subscribe:         newNewsletterOpHandler(uc, notification.NewsletterOpSubscribe),
		Demote:            newNewsletterOpHandler(uc, notification.NewsletterOpDemote),
		ChangeOwner:       newNewsletterOpHandler(uc, notification.NewsletterOpChangeOwner),
		Delete:            newNewsletterOpHandler(uc, notification.NewsletterOpDelete),
		AdminInvite:       newNewsletterOpHandler(uc, notification.NewsletterOpAdminInvite),
		AdminInviteAccept: newNewsletterOpHandler(uc, notification.NewsletterOpAdminInviteAccept),
		AdminInviteRevoke: newNewsletterOpHandler(uc, notification.NewsletterOpAdminInviteRevoke),
	}
}

func (h *newsletterOpHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, ok := sessionUser(w, r)
	if !ok {
		return
	}

	var body newsletterBody
	if !decodeAndRespond(w, r, &body) {
		return
	}

	req, err := body.toRequest(h.op)
	if err != nil {
		hlog.FromRequest(r).Warn().Err(err).Str("route", r.URL.Path).Msg("newsletter request rejected")
		customhttp.RespondJSON(w, http.StatusBadRequest, nil, err)
		return
	}

	rsp, err := h.uc.Execute(r.Context(), id, req)
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).Str("route", r.URL.Path).Msg("newsletter operation failed")
		// 500 e' o piso, nao a decisao: RespondJSON troca-o pelo status da
		// categoria quando o erro e' um apperr (response.go:52). Repetir esse
		// mapeamento aqui daria duas fontes de verdade para o mesmo status.
		customhttp.RespondJSON(w, http.StatusInternalServerError, nil, err)
		return
	}
	customhttp.RespondJSON(w, http.StatusOK, rsp.Data, nil)
}

// toRequest converts the wire body into the use case request. The only
// conversion that can fail is `since`, because RFC 3339 is a format the client
// can get wrong — everything else is a straight copy and the use case decides
// whether it is present.
func (b newsletterBody) toRequest(op notification.NewsletterOp) (notification.NewsletterRequest, error) {
	req := notification.NewsletterRequest{
		Op:          op,
		JID:         domain.JID(b.JID),
		Invite:      b.Invite,
		Name:        b.Name,
		Description: b.Description,
		Picture:     b.Picture,
		Mute:        b.Mute,
		Count:       b.Count,
		Before:      b.Before,
		After:       b.After,
		ServerIDs:   b.ServerIDs,
		ServerID:    b.ServerID,
		Reaction:    b.Reaction,
		MessageID:   b.MessageID,
		UserJID:     domain.JID(b.UserJID),
		ConfirmJID:  domain.JID(b.ConfirmJID),
	}
	if b.Since != "" {
		since, err := time.Parse(time.RFC3339, b.Since)
		if err != nil {
			return notification.NewsletterRequest{}, err
		}
		req.Since = since
	}
	return req, nil
}
