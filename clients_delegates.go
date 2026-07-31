package wuzapi

import "wuzapi/internal/infrastructure/whatsmeow"

// ClientManager wraps the internal whatsmeow ClientManager with typed MyClient.
type ClientManager = whatsmeow.ClientManager
var NewClientManager = whatsmeow.NewClientManager
