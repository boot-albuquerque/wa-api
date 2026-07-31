package infrastructure

import "sync"

// UserSemaphoreManager manages per-user semaphore pools for concurrency control.
type UserSemaphoreManager struct {
	pools sync.Map
}

// NewUserSemaphoreManager creates a new UserSemaphoreManager.
func NewUserSemaphoreManager(maxConcurrent int) *UserSemaphoreManager {
	return &UserSemaphoreManager{}
}

// ForUser returns a semaphore pool for the given user ID.
// LoadOrStore provides an atomic way to get or create a semaphore.
func (usm *UserSemaphoreManager) ForUser(userID string, maxConcurrent int) chan struct{} {
	pool, _ := usm.pools.LoadOrStore(userID, make(chan struct{}, maxConcurrent))
	return pool.(chan struct{})
}
