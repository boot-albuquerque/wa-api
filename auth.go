package wuzapi

import (
	"strings"

	"github.com/rs/zerolog/log"
)

// Values is a simple string map used to carry user attributes through the
// request context. Keys are set during authentication and consumed by
// downstream handlers.
type Values struct {
	m map[string]string
}

// Get returns the value for a key or the empty string when missing.
func (v Values) Get(key string) string { return v.m[key] }

// respondJSON delegated to middleware.RespondJSON

// authAdmin delegated to middleware.AuthAdmin

// authAlice delegated to middleware.AuthAlice

// resolveConnectEvents decides which event-subscription string to persist when a
// client (re)connects. With no subscribe list the existing subscriptions are
// preserved instead of being overwritten with an empty value (issue #305);
// changed reports whether the stored value needs updating.
func resolveConnectEvents(subscribe []string, existing string) (eventstring string, changed bool) {
	if len(subscribe) < 1 {
		return existing, false
	}
	var subscribedEvents []string
	for _, arg := range subscribe {
		if !Find(supportedEventTypes, arg) {
			log.Warn().Str("Type", arg).Msg("Event type discarded")
			continue
		}
		if !Find(subscribedEvents, arg) {
			subscribedEvents = append(subscribedEvents, arg)
		}
	}
	resolved := strings.Join(subscribedEvents, ",")
	return resolved, resolved != existing
}
