package handlers

// userInfo interface matches the type from profile_handler.go
type userInfo interface {
	Get(key string) string
}

// Common error definitions used across handlers
var (
	errUnauthorized     = &simpleErr{"unauthorized"}
	errMissingSessionID = &simpleErr{"missing session id"}
	errMissingPhone     = &simpleErr{"missing phone"}
	errMissingJID       = &simpleErr{"missing jid"}
	errMissingID        = &simpleErr{"missing ID"}
	errDecodePayload    = &simpleErr{"could not decode payload"}
)

type simpleErr struct {
	msg string
}

func (e simpleErr) Error() string {
	return e.msg
}
