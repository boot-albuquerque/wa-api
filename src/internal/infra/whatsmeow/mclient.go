package whatsmeow

import (
	"go.mau.fi/whatsmeow"
)

// MyClientImpl is the concrete implementation of MyClient interface.
// It is designed to wrap a WhatsApp client and associated metadata.
// Note: The full implementation in wmiau.go also includes *sqlx.DB and *server
// fields for direct access to the database and HTTP server. This interface-focused
// struct definition supports dependency injection patterns.
type MyClientImpl struct {
	WAClient       *whatsmeow.Client
	eventHandlerID uint32
	userID         string
	token          string
}

// GetWAClient returns the underlying WhatsApp client.
func (mc *MyClientImpl) GetWAClient() *whatsmeow.Client {
	return mc.WAClient
}

// GetUserID returns the user ID associated with this client.
func (mc *MyClientImpl) GetUserID() string {
	return mc.userID
}

// Verify that MyClientImpl implements the MyClient interface.
var _ MyClient = (*MyClientImpl)(nil)
