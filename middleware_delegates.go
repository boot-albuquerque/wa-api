package wuzapi

import "wuzapi/internal/interfaces/http/middleware"

// Middleware delegates — thin bridges to internal/interfaces/http/middleware.
var respondJSON = middleware.RespondJSON
var authAdmin = middleware.AuthAdmin
var authAlice = middleware.AuthAlice
