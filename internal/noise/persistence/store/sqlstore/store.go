// Package sqlstore contains an SQL-backed implementation of the interfaces in the store package.
package sqlstore

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"sync"

	"go.mau.fi/util/exsync"

	"wa-api/internal/noise/persistence/store"
	"wa-api/internal/noise/protocol/types"
)

// ErrInvalidLength is returned by some database getters if the database returned a byte array with an unexpected length.
// This should be impossible, as the database schema contains CHECK()s for all the relevant columns.
var ErrInvalidLength = errors.New("database returned byte array with illegal length")

// PostgresArrayWrapper is a function to wrap array values before passing them to the sql package.
//
// When using github.com/lib/pq, you should set
//
//	noise.PostgresArrayWrapper = pq.Array
var PostgresArrayWrapper func(any) interface {
	driver.Valuer
	sql.Scanner
}

type SQLStore struct {
	*Container
	JID string

	preKeyLock sync.Mutex

	contactCache     map[types.JID]*types.ContactInfo
	contactCacheLock sync.Mutex

	migratedPNSessionsCache *exsync.Set[string]
}

// NewSQLStore creates a new SQLStore with the given database container and user JID.
// It contains implementations of all the different stores in the store package.
//
// In general, you should use Container.NewDevice or Container.GetDevice instead of this.
func NewSQLStore(c *Container, jid types.JID) *SQLStore {
	return &SQLStore{
		Container:    c,
		JID:          jid.String(),
		contactCache: make(map[types.JID]*types.ContactInfo),

		migratedPNSessionsCache: exsync.NewSet[string](),
	}
}

var _ store.AllSessionSpecificStores = (*SQLStore)(nil)
