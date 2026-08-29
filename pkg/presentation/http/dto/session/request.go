package session

import "wa-api/pkg/domain"

// The request DTOs of the session family.
//
// Validate is deliberately THIN here, and the thinness is a decision, not an
// omission: every check this family needs already lives in its use case
// (missing_phone in usecase/session/pair_phone.go, missing_body in
// set_status_message.go, the media union in usecase/status/*). Duplicating them
// at the boundary would give the same request TWO places to answer 400 from,
// which drifts the moment one of the two is edited. The DTOs exist here for the
// wire NAMES and for ToDomain; the taxonomy stays where it is.
//
// A Validate that has nothing to say returns nil and says so in a comment. It
// is kept for shape, so that a future check has an obvious home instead of
// landing in the handler.

// PairPhoneRequest is the body of POST /session/pairphone.
//
// The key was `Phone` — PascalCase on the wire. `engine` is mandatory since
// 2026-08-28 — see pkg/pairing for the order it is validated in
// (invalid_engine -> engine_mismatch -> capability_not_supported ->
// engine_unavailable) and HOUSEKEEP F273 for the defect this closes.
type PairPhoneRequest struct {
	Phone  string `json:"phone"`
	Engine string `json:"engine"`
}

// Validate: the empty phone is refused by PairPhoneUseCase with the
// `missing_phone` code, and that code is public contract. The empty/invalid
// engine is refused by pkg/pairing before the use case is even built —
// keeping this Validate() thin, per the file's own doc comment above.
func (r PairPhoneRequest) Validate() error { return nil }

// ToDomain produces the use case input.
func (r PairPhoneRequest) ToDomain() domain.PairPhoneRequest {
	return domain.PairPhoneRequest{Phone: r.Phone, Engine: r.Engine}
}

// SetStatusMessageRequest is the body of POST /session/statusmessage.
//
// The key was `Body`.
type SetStatusMessageRequest struct {
	Body string `json:"body"`
}

// Validate: the empty body is refused by SetStatusMessageUseCase with
// `missing_body`.
func (r SetStatusMessageRequest) Validate() error { return nil }

// ToDomain produces the use case input.
func (r SetStatusMessageRequest) ToDomain() domain.SetStatusMessageRequest {
	return domain.SetStatusMessageRequest{Body: r.Body}
}

// RequestHistorySyncRequest is the body of POST /session/historysync.
//
// Every field was already snake_case; what changed is that they no longer come
// from a domain struct. The whole body is optional — the handler tolerates EOF
// — so nothing here is a pointer: zero and absent mean the same thing for a
// count and for a cursor.
type RequestHistorySyncRequest struct {
	Count              int    `json:"count"`
	ChatJID            string `json:"chat_jid"`
	OldestMsgID        string `json:"oldest_msg_id"`
	OldestMsgFromMe    bool   `json:"oldest_msg_from_me"`
	OldestMsgTimestamp int64  `json:"oldest_msg_timestamp"`
}

// Validate: RequestHistorySyncUseCase owns the bounds of Count.
func (r RequestHistorySyncRequest) Validate() error { return nil }

// ToDomain produces the use case input.
func (r RequestHistorySyncRequest) ToDomain() domain.RequestHistorySyncRequest {
	return domain.RequestHistorySyncRequest{
		Count:              r.Count,
		ChatJid:            r.ChatJID,
		OldestMsgID:        r.OldestMsgID,
		OldestMsgFromMe:    r.OldestMsgFromMe,
		OldestMsgTimestamp: r.OldestMsgTimestamp,
	}
}

// SyncContactRosterRequest is the body of POST /user/contacts/sync.
type SyncContactRosterRequest struct {
	// Mode picks the cost of the pull: "if_unsynced", "incremental" or "full".
	Mode string `json:"mode"`
}

// Validate: SyncContactRosterUseCase owns the set of accepted modes.
func (r SyncContactRosterRequest) Validate() error { return nil }

// ToDomain produces the use case input.
func (r SyncContactRosterRequest) ToDomain() domain.SyncContactRosterRequest {
	return domain.SyncContactRosterRequest{Mode: r.Mode}
}

// PublishStatusImageRequest is the body of POST /status/set/image.
//
// Keys were `Image`, `Caption`, `Id`, `MimeType`, `JPEGThumbnail`.
type PublishStatusImageRequest struct {
	Image         string `json:"image"`
	Caption       string `json:"caption"`
	ID            string `json:"id"`
	MimeType      string `json:"mime_type"`
	JPEGThumbnail []byte `json:"jpeg_thumbnail"`
}

// Validate: PublishStatusImageUseCase owns the image union (data URI or URL).
func (r PublishStatusImageRequest) Validate() error { return nil }

// ToDomain produces the use case input.
func (r PublishStatusImageRequest) ToDomain() domain.PublishStatusImageRequest {
	return domain.PublishStatusImageRequest{
		Image:         r.Image,
		Caption:       r.Caption,
		ID:            r.ID,
		MimeType:      r.MimeType,
		JPEGThumbnail: r.JPEGThumbnail,
	}
}

// PublishStatusVideoRequest is the body of POST /status/set/video.
type PublishStatusVideoRequest struct {
	Video         string `json:"video"`
	Caption       string `json:"caption"`
	ID            string `json:"id"`
	MimeType      string `json:"mime_type"`
	JPEGThumbnail []byte `json:"jpeg_thumbnail"`
}

// Validate: PublishStatusVideoUseCase owns the video union.
func (r PublishStatusVideoRequest) Validate() error { return nil }

// ToDomain produces the use case input.
func (r PublishStatusVideoRequest) ToDomain() domain.PublishStatusVideoRequest {
	return domain.PublishStatusVideoRequest{
		Video:         r.Video,
		Caption:       r.Caption,
		ID:            r.ID,
		MimeType:      r.MimeType,
		JPEGThumbnail: r.JPEGThumbnail,
	}
}

// PublishStatusAudioRequest is the body of POST /status/set/audio.
//
// The mime key was `mimetype` here and `MimeType` on the two routes above —
// three routes, two spellings, for the same concept. It is `mime_type` on all
// three now.
type PublishStatusAudioRequest struct {
	Audio    string `json:"audio"`
	ID       string `json:"id"`
	MimeType string `json:"mime_type"`
}

// Validate: PublishStatusAudioUseCase owns the audio union.
func (r PublishStatusAudioRequest) Validate() error { return nil }

// ToDomain produces the use case input.
func (r PublishStatusAudioRequest) ToDomain() domain.PublishStatusAudioRequest {
	return domain.PublishStatusAudioRequest{
		Audio:    r.Audio,
		ID:       r.ID,
		MimeType: r.MimeType,
	}
}
