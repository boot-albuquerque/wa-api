package main

import "wuzapi/internal/infrastructure/constants"

// supportedEventTypes is a direct alias for the constants package slice.
var supportedEventTypes = constants.SupportedEventTypes

// isValidEventType reports whether the named event type is recognized.
func isValidEventType(name string) bool {
	return constants.IsValidEventType(name)
}
