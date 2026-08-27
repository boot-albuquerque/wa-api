package handlers

import (
	"net/http"

	"github.com/rs/zerolog/hlog"

	"wa-api/pkg/application/usecase/notification"
	"wa-api/pkg/domain"
	customhttp "wa-api/pkg/presentation/http"
	dtonewsletter "wa-api/pkg/presentation/http/dto/newsletter"
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

	var body dtonewsletter.NewsletterRequest
	if !decodeAndRespond(w, r, &body) {
		return
	}
	if err := body.Validate(); err != nil {
		hlog.FromRequest(r).Warn().Err(err).Str("route", r.URL.Path).Msg("newsletter request rejected")
		customhttp.RespondJSON(w, http.StatusBadRequest, nil, err)
		return
	}

	rsp, err := h.uc.Execute(r.Context(), id, newsletterRequestFor(h.op, body.ToDomain()))
	if err != nil {
		hlog.FromRequest(r).Error().Err(err).Str("route", r.URL.Path).Msg("newsletter operation failed")
		// 500 e' o piso, nao a decisao: RespondJSON troca-o pelo status da
		// categoria quando o erro e' um apperr (response.go:52). Repetir esse
		// mapeamento aqui daria duas fontes de verdade para o mesmo status.
		customhttp.RespondJSON(w, http.StatusInternalServerError, nil, err)
		return
	}
	h.respond(w, rsp)
}

// respond serves the response shape that this handler's operation answers with.
//
// FOUR SHAPES AND NOT ONE UNION, because they answer four different questions:
// a union would make every route emit the keys of every other — `messages: []`
// on a follow, `newsletter: null` on a delete — with no way for a caller to tell
// "not applicable" from "empty".
//
// The RespondJSON call is repeated per branch rather than hoisted, and the
// repetition is what the architectural gate reads: it classifies by the FORM of
// the serialized expression, so `dtonewsletter.Present…` at each call site is
// what proves the route no longer hands a domain value to the encoder. One
// hoisted call over a variable would classify by that variable's name, which
// proves nothing (respondjson_ledger_test.go).
func (h *newsletterOpHandler) respond(w http.ResponseWriter, rsp *notification.NewsletterResult) {
	if rsp == nil {
		customhttp.RespondJSON(w, http.StatusOK, dtonewsletter.PresentNewsletterAck(""), nil)
		return
	}
	switch h.op {
	case notification.NewsletterOpCreate,
		notification.NewsletterOpInfo,
		notification.NewsletterOpInfoInvite:
		customhttp.RespondJSON(w, http.StatusOK, dtonewsletter.PresentNewsletterInfo(rsp.Metadata), nil)
	case notification.NewsletterOpMessages,
		notification.NewsletterOpUpdates:
		customhttp.RespondJSON(w, http.StatusOK, dtonewsletter.PresentNewsletterMessages(rsp.Messages), nil)
	case notification.NewsletterOpSubscribe:
		customhttp.RespondJSON(w, http.StatusOK, dtonewsletter.PresentNewsletterSubscribe(rsp.Status, rsp.Duration), nil)
	default:
		// The acknowledgement is the right answer for every operation that
		// changes something and reports no payload, which is what a newly added
		// write operation is — so the default is correct rather than a gap.
		customhttp.RespondJSON(w, http.StatusOK, dtonewsletter.PresentNewsletterAck(rsp.Status), nil)
	}
}

// newsletterRequestFor turns the validated body into the use case request.
//
// The assignment lives HERE and not in the DTO package because the import rule
// is one-way: pkg/presentation/http/dto may import pkg/domain and nothing above
// it, so a ToDomain that returned a notification.NewsletterRequest would put an
// application type inside the wire layer (docs/HTTP-DTO-CONVENTIONS.md §3).
func newsletterRequestFor(op notification.NewsletterOp, in dtonewsletter.NewsletterInput) notification.NewsletterRequest {
	return notification.NewsletterRequest{
		Op:          op,
		JID:         domain.JID(in.JID),
		Invite:      in.Invite,
		Name:        in.Name,
		Description: in.Description,
		Picture:     in.Picture,
		Mute:        in.Mute,
		Count:       in.Count,
		Before:      in.Before,
		After:       in.After,
		Since:       in.Since,
		ServerIDs:   in.ServerIDs,
		ServerID:    in.ServerID,
		Reaction:    in.Reaction,
		MessageID:   in.MessageID,
		UserJID:     domain.JID(in.UserJID),
		ConfirmJID:  domain.JID(in.ConfirmJID),
	}
}
