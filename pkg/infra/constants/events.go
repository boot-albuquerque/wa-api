package constants

import "wa-api/pkg/domain"

// SupportedEventTypes is a defensive copy of domain.SupportedEventTypes.
// Aliasing (var X = domain.X) would share the backing array, so a write
// to one slice header's index would silently corrupt the other. A copy
// costs 48 pointers once at init and eliminates that class of bug.
var SupportedEventTypes []string

func init() {
	SupportedEventTypes = make([]string, len(domain.SupportedEventTypes))
	copy(SupportedEventTypes, domain.SupportedEventTypes)
}

// IsValidEventType delegates to domain.IsValidEventType — single source of truth.
func IsValidEventType(name string) bool {
	return domain.IsValidEventType(name)
}
