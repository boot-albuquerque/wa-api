package registry

// SetPollOptions remembers the plaintext options of a poll we just sent so
// that incoming votes (which arrive as SHA-256 hashes of the option text)
// can be resolved back to readable strings.
func (cm *ClientManager) SetPollOptions(userID, msgID string, options []string) {
	cm.Lock()
	defer cm.Unlock()
	if cm.pollOptions[userID] == nil {
		cm.pollOptions[userID] = make(map[string][]string)
	}
	stored := make([]string, len(options))
	copy(stored, options)
	cm.pollOptions[userID][msgID] = stored
}

// GetPollOptions returns the plaintext options associated with a poll
// message, or nil if none were recorded (e.g. wa-api restarted after the
// poll was sent).
func (cm *ClientManager) GetPollOptions(userID, msgID string) []string {
	cm.RLock()
	defer cm.RUnlock()
	if byUser := cm.pollOptions[userID]; byUser != nil {
		return byUser[msgID]
	}
	return nil
}
