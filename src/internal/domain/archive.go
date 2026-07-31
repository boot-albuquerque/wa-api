package domain

// ArchiveChatRequest represents a request to archive or unarchive a chat
type ArchiveChatRequest struct {
	Jid     string `json:"jid"`
	Archive bool   `json:"archive"`
}

// ArchiveChatResult represents the result of archiving/unarchiving a chat
type ArchiveChatResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}
