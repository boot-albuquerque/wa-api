package domain

// DeleteUserCompleteResult is what the "delete everything about this user"
// use case reports back: which user was removed, and what was done.
//
// It used to carry Code, Success and Data as well, with `json` tags — a
// SECOND copy of the HTTP envelope, living in the domain. The handler then
// answered with `rsp.Code` as the HTTP status, which meant the domain layer
// was choosing the status line. Both are gone: the envelope is produced by
// RespondJSON and by nothing else (docs/HTTP-DTO-CONVENTIONS.md §7).
type DeleteUserCompleteResult struct {
	User UserDeleteData

	// Details is the human-readable summary of what the removal did. It was
	// computed and then dropped on the floor: the handler served only Data.
	Details string
}

// UserDeleteData identifies the user that was removed. Read BEFORE the
// deletion, because afterwards there is no row left to read it from.
type UserDeleteData struct {
	ID   string
	Name string
	JID  string
}
