package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

// Session ownership by lease (ADR-0005, D2).
//
// A WhatsApp session is stateful — the socket lives inside one process — and
// F89 measured what happens without coordination: two replicas holding the
// same session do NOT fight in a loop. WhatsApp sends a single
// `StreamReplaced`, the loser's session dies and never reconnects, and the
// database keeps reporting `connected=1`. Process alive, liveness green,
// session dead.
//
// This repository is what decides, BEFORE connecting, which process has the
// right to the session.
//
// POSTGRES ONLY, which is why it uses `now()` and `make_interval` with no
// dialect branch: `multi` mode requires Postgres (D1), and `single` mode has
// no ownership to coordinate. The table exists in SQLite only so the two
// schemas do not diverge.

// ErrLeaseUnavailable separates "I lost ownership" from "I could not reach the
// database". The distinction drives what a failed renewal does: ownership
// taken by someone else forces us to drop the session, an unreachable database
// does NOT — killing a healthy session over a network blip is worse than the
// problem this mechanism solves.
var ErrLeaseUnavailable = errors.New("database unavailable for lease operation")

// SessionLeaseRepository manages the `session_leases` table.
type SessionLeaseRepository struct {
	db *sqlx.DB
}

func NewSessionLeaseRepository(db *sqlx.DB) *SessionLeaseRepository {
	return &SessionLeaseRepository{db: db}
}

// claimStatement takes or renews ownership in a single round trip.
//
// The deadline is computed by the DATABASE clock (`now() + make_interval`),
// never by the process clock. Across N replicas, skewed clocks would let one
// believe another's lease had expired when it had not — and two simultaneous
// owners is precisely what this mechanism exists to prevent.
//
// The `OR owner_id = EXCLUDED.owner_id` clause makes the operation idempotent
// for the current owner, which is what lets Claim double as renewal.
//
// `owner_addr` viaja junto com a posse, e não numa escrita separada
// (ADR-0007, decisão 1): quem for rotear lê as duas respostas — quem é o dono e
// onde ele está — da mesma linha. Duas escritas abririam uma janela em que a
// posse já mudou e o endereço ainda é o do dono anterior, que é exatamente o
// tipo de divergência silenciosa que este desenho existe para não ter.
const claimStatement = `
	INSERT INTO session_leases (user_id, owner_id, owner_addr, expires_at)
	VALUES ($1, $2, $3, now() + make_interval(secs => $4))
	ON CONFLICT (user_id) DO UPDATE
	   SET owner_id = EXCLUDED.owner_id,
	       owner_addr = EXCLUDED.owner_addr,
	       expires_at = EXCLUDED.expires_at
	 WHERE session_leases.expires_at < now()
	    OR session_leases.owner_id = EXCLUDED.owner_id`

// releaseStatement drops ownership.
//
// The `AND owner_id = $2` guard stops one process from deleting another's
// lease: without it, a slow shutdown could release ownership the next replica
// had already taken, and both would believe they own the session.
const releaseStatement = `DELETE FROM session_leases WHERE user_id = $1 AND owner_id = $2`

const currentOwnerStatement = `SELECT owner_id, owner_addr, expires_at FROM session_leases WHERE user_id = $1`

// LeaseHolder is who holds a session and where to reach them.
//
// A struct rather than three return values because the three are only ever
// meaningful together: an address without the owner it belongs to cannot be
// verified, and verification is what makes a stale address safe (ADR-0007,
// decisão 1).
type LeaseHolder struct {
	OwnerID   string
	OwnerAddr string
	ExpiresAt time.Time
}

// Claim attempts to take (or renew) ownership of a session.
//
// Returns true when this owner now holds the session. Returns false with no
// error when ANOTHER owner holds a valid lease — that is not a failure, it is
// the answer.
func (r *SessionLeaseRepository) Claim(ctx context.Context, userID, ownerID, ownerAddr string, ttl time.Duration) (bool, error) {
	res, err := r.db.ExecContext(ctx, claimStatement, userID, ownerID, ownerAddr, ttl.Seconds())
	if err != nil {
		return false, fmt.Errorf("%w: %v", ErrLeaseUnavailable, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("%w: %v", ErrLeaseUnavailable, err)
	}
	return affected > 0, nil
}

// Release drops ownership.
//
// Called on graceful shutdown so failover does not have to wait out the TTL:
// F89 measured session re-establishment at 1.2s to 2.3s, so the TTL dominates
// downtime in the common case.
func (r *SessionLeaseRepository) Release(ctx context.Context, userID, ownerID string) error {
	if _, err := r.db.ExecContext(ctx, releaseStatement, userID, ownerID); err != nil {
		return fmt.Errorf("%w: %v", ErrLeaseUnavailable, err)
	}
	return nil
}

// CurrentOwner reports who holds the lease, where to reach them, and until when.
//
// The bool separates "there is no lease" from "the lease is held by someone
// with an empty address": both produce a zero-ish LeaseHolder, and they demand
// opposite actions from a router — claim it, versus refuse to forward. Absence
// is not an error, but it is not the same as presence either.
func (r *SessionLeaseRepository) CurrentOwner(ctx context.Context, userID string) (LeaseHolder, bool, error) {
	var holder LeaseHolder
	err := r.db.QueryRowxContext(ctx, currentOwnerStatement, userID).
		Scan(&holder.OwnerID, &holder.OwnerAddr, &holder.ExpiresAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return LeaseHolder{}, false, nil
		}
		return LeaseHolder{}, false, fmt.Errorf("%w: %v", ErrLeaseUnavailable, err)
	}
	return holder, true, nil
}
