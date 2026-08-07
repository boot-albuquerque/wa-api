package registry

import (
	wanoise "wa-api/internal/wa-noise"
)

// MyClient defines the interface for WhatsApp client wrappers.
type MyClient interface {
	GetWAClient() *wanoise.Client
	GetUserID() string
}

// SetMyClient stores a MyClient instance.
func (cm *ClientManager) SetMyClient(userID string, client MyClient) {
	cm.Lock()
	defer cm.Unlock()
	cm.myClients[userID] = client
}

// GetMyClient retrieves a MyClient instance.
func (cm *ClientManager) GetMyClient(userID string) MyClient {
	cm.RLock()
	defer cm.RUnlock()
	return cm.myClients[userID]
}

// DeleteMyClient removes a user's MyClient entry and clears any cached
// poll options associated with that user.
func (cm *ClientManager) DeleteMyClient(userID string) {
	cm.Lock()
	defer cm.Unlock()
	delete(cm.myClients, userID)
	delete(cm.pollOptions, userID)
}
