package port

import "context"

// UserInfoRepublisher drops a user's cached info so the next read reloads it
// from the database.
//
// WHY THIS EXISTS. This process caches user info in two places, and the
// admin edit path used to write the database and touch neither:
//
//   - the user-id cache holds entries under NO EXPIRATION, so a stale value
//     there is permanent for the life of the process — `saveMessageHistory`
//     reads the history limit from it, and a stale 0 means inbound messages
//     are silently never persisted;
//   - the token cache authenticates requests, so a token replaced by an admin
//     kept working until its TTL ran out.
//
// Both were measured against a live server in HOUSEKEEP F200/F201: a
// `PUT /admin/users/{id}` answering `success: true` had no effect at all
// until the process restarted.
//
// The session-config path already published its writes through a dedicated
// port; this is the same seam for the admin path, which is the one that
// changes EVERY field at once and therefore republishes the whole entry
// instead of patching one field.
type UserInfoRepublisher interface {
	// RepublishUser invalidates every cached entry that belongs to userID —
	// the one keyed by user id and every one keyed by a token of that user,
	// including tokens that were just replaced.
	//
	// It reports no error on purpose: a cache that could not be refreshed is
	// not a reason to fail an edit the database already accepted. The
	// implementation logs, and the next read reloads from the database.
	RepublishUser(ctx context.Context, userID string)
}
