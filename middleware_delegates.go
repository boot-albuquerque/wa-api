package wuzapi

import "wuzapi/internal/interfaces/http/middleware"

// Auth middleware delegates.
type Values = middleware.Values
var respondJSON = middleware.RespondJSON
var authAdmin = middleware.AuthAdmin
var authAlice = middleware.AuthAlice
func resolveConnectEvents(subscribe []string, existing string) (string, bool) { return middleware.ResolveConnectEvents(Find, supportedEventTypes, subscribe, existing) }
