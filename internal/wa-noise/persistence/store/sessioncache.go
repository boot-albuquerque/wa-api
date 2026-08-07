package store

import (
	"context"
	"fmt"

	"github.com/rs/zerolog"
	"go.mau.fi/libsignal/state/record"

	"go.mau.fi/util/exsync"
)

type contextKey int

const (
	contextKeySessionCache contextKey = iota
)

type sessionCacheEntry struct {
	Dirty  bool
	Found  bool
	Record *record.Session
}

type sessionCache = exsync.Map[string, sessionCacheEntry]

func getSessionCache(ctx context.Context) *sessionCache {
	if ctx == nil {
		return nil
	}
	val := ctx.Value(contextKeySessionCache)
	if val == nil {
		return nil
	}
	if cache, ok := val.(*sessionCache); ok {
		return cache
	}
	return nil
}

func getCachedSession(ctx context.Context, addr string) *record.Session {
	cache := getSessionCache(ctx)
	if cache == nil {
		return nil
	}
	sess, ok := cache.Get(addr)
	if !ok {
		return nil
	}
	return sess.Record
}

func putCachedSession(ctx context.Context, addr string, record *record.Session) bool {
	cache := getSessionCache(ctx)
	if cache == nil {
		return false
	}
	cache.Set(addr, sessionCacheEntry{
		Dirty:  true,
		Found:  true,
		Record: record,
	})
	return true
}

func (device *Device) WithCachedSessions(ctx context.Context, addresses []string) (map[string]bool, context.Context, error) {
	if len(addresses) == 0 {
		return nil, ctx, nil
	}

	sessions, err := device.Sessions.GetManySessions(ctx, addresses)
	if err != nil {
		return nil, ctx, fmt.Errorf("failed to prefetch sessions: %w", err)
	}
	wrapped := make(map[string]sessionCacheEntry, len(sessions))
	existingSessions := make(map[string]bool, len(sessions))
	for addr, rawSess := range sessions {
		var sessionRecord *record.Session
		var found bool
		if rawSess == nil {
			sessionRecord = record.NewSession(SignalProtobufSerializer.Session, SignalProtobufSerializer.State)
		} else {
			found = true
			sessionRecord, err = record.NewSessionFromBytes(rawSess, SignalProtobufSerializer.Session, SignalProtobufSerializer.State)
			if err != nil {
				zerolog.Ctx(ctx).Err(err).
					Str("address", addr).
					Msg("Failed to deserialize session")
				continue
			}
		}
		existingSessions[addr] = found
		wrapped[addr] = sessionCacheEntry{Record: sessionRecord, Found: found}
	}

	ctx = context.WithValue(ctx, contextKeySessionCache, (*sessionCache)(exsync.NewMapWithData(wrapped)))
	return existingSessions, ctx, nil
}

func (device *Device) PutCachedSessions(ctx context.Context) error {
	cache := getSessionCache(ctx)
	if cache == nil {
		return nil
	}
	dirtySessions := make(map[string][]byte)
	for addr, item := range cache.Iter() {
		if !item.Dirty {
			continue
		}
		// Record.Serialize() entra em PANIC num record "fresco": um
		// record.NewSession() tem localIdentityPublic, remoteIdentityPublic,
		// senderBaseKey e senderChain nil, e State.structure() os desreferencia
		// sem checar (F22 em HOUSEKEEP.md).
		//
		// Nao explodia em producao porque Dirty so' e' setado por
		// putCachedSession, que o libsignal chama DEPOIS do handshake — quando
		// o state ja' esta' preenchido. Ou seja, a protecao existia, mas era
		// uma invariante acidental: nada no codigo a declarava, e qualquer
		// mudanca no libsignal ou no fluxo de handshake a quebraria em silencio.
		//
		// IsFresh() e' o predicado exportado que torna a invariante explicita —
		// true para NewSession, false para NewSessionFromBytes. A entrada da
		// F22 afirmava que ele nao existia; existe, em SessionRecord.go:113.
		if item.Record.IsFresh() {
			continue
		}
		dirtySessions[addr] = item.Record.Serialize()
	}
	if len(dirtySessions) > 0 {
		err := device.Sessions.PutManySessions(ctx, dirtySessions)
		if err != nil {
			return fmt.Errorf("failed to store cached sessions: %w", err)
		}
	}
	cache.Clear()
	return nil
}
