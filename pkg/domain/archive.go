package domain

// ArchiveChatRequest represents a request to archive or unarchive a chat
type ArchiveChatRequest struct {
	ChatTarget
	Jid     string `json:"jid"`
	Archive bool   `json:"archive"`
}

func (r *ArchiveChatRequest) ResolveChat() { ResolveChatField(&r.Jid, r.ChatAlias) }

// ArchiveChatResult represents the result of archiving/unarchiving a chat
type ArchiveChatResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}
