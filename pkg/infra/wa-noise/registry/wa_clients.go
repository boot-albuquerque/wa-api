package registry

import (
	wanoise "wa-api/internal/wa-noise"
)

// Ciclo de vida dos *wa-noise.Client por usuário.

func (cm *ClientManager) SetWaNoiseClient(userID string, client *wanoise.Client) {
	cm.Lock()
	defer cm.Unlock()
	cm.wanoiseClients[userID] = client
}

func (cm *ClientManager) GetWaNoiseClient(userID string) *wanoise.Client {
	cm.RLock()
	defer cm.RUnlock()
	return cm.wanoiseClients[userID]
}

func (cm *ClientManager) DeleteWaNoiseClient(userID string) {
	cm.Lock()
	defer cm.Unlock()
	delete(cm.wanoiseClients, userID)
}

// GetAllClients returns a snapshot of all wa-noise clients (read-only copy of keys)
func (cm *ClientManager) GetAllClients() map[string]*wanoise.Client {
	cm.RLock()
	defer cm.RUnlock()
	result := make(map[string]*wanoise.Client)
	for k, v := range cm.wanoiseClients {
		result[k] = v
	}
	return result
}

// Getwa-noiseClientsCount returns the count of wa-noise clients
func (cm *ClientManager) GetWaNoiseClientsCount() int {
	cm.RLock()
	defer cm.RUnlock()
	return len(cm.wanoiseClients)
}

// Iteratewa-noiseClients safely iterates over all wa-noise clients with a callback
func (cm *ClientManager) IterateWaNoiseClients(callback func(*wanoise.Client) bool) {
	cm.RLock()
	defer cm.RUnlock()
	for _, client := range cm.wanoiseClients {
		if !callback(client) {
			break
		}
	}
}
