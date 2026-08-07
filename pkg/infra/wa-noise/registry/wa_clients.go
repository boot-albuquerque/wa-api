package registry

import (
	whatsmeow "wa-api/internal/wa-noise/core"
)

// Ciclo de vida dos *whatsmeow.Client por usuário.

func (cm *ClientManager) SetWhatsmeowClient(userID string, client *whatsmeow.Client) {
	cm.Lock()
	defer cm.Unlock()
	cm.whatsmeowClients[userID] = client
}

func (cm *ClientManager) GetWhatsmeowClient(userID string) *whatsmeow.Client {
	cm.RLock()
	defer cm.RUnlock()
	return cm.whatsmeowClients[userID]
}

func (cm *ClientManager) DeleteWhatsmeowClient(userID string) {
	cm.Lock()
	defer cm.Unlock()
	delete(cm.whatsmeowClients, userID)
}

// GetAllClients returns a snapshot of all whatsmeow clients (read-only copy of keys)
func (cm *ClientManager) GetAllClients() map[string]*whatsmeow.Client {
	cm.RLock()
	defer cm.RUnlock()
	result := make(map[string]*whatsmeow.Client)
	for k, v := range cm.whatsmeowClients {
		result[k] = v
	}
	return result
}

// GetWhatsmeowClientsCount returns the count of whatsmeow clients
func (cm *ClientManager) GetWhatsmeowClientsCount() int {
	cm.RLock()
	defer cm.RUnlock()
	return len(cm.whatsmeowClients)
}

// IterateWhatsmeowClients safely iterates over all whatsmeow clients with a callback
func (cm *ClientManager) IterateWhatsmeowClients(callback func(*whatsmeow.Client) bool) {
	cm.RLock()
	defer cm.RUnlock()
	for _, client := range cm.whatsmeowClients {
		if !callback(client) {
			break
		}
	}
}
