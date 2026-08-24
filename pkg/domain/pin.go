package domain

// PinChatRequest represents a request to pin or unpin a chat.
//
// This pins the CONVERSATION in the chat list, not a message inside it.
// Pinning a message is a different mechanism entirely (waE2E.PinInChatMessage,
// sent as a message with its own ID) and is deliberately NOT exposed here.
//
// Observable side effect we inherit and do not control: archiving a chat also
// UNPINS it. See internal/wa-noise/protocol/appstate/patch_builders_chat.go,
// the BuildArchive doc comment: "Archiving a chat will also unpin it
// automatically." A caller that pins and then archives ends with an unpinned
// chat, and nothing in this API reports that.
type PinChatRequest struct {
	Jid string `json:"jid"`
	Pin bool   `json:"pin"`
}

// PinChatResult represents the result of pinning/unpinning a chat.
type PinChatResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}
