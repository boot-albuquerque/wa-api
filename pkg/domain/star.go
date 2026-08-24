package domain

// StarMessageRequest is the HTTP payload for POST /message/star.
type StarMessageRequest struct {
	Chat      string `json:"chat"`
	Sender    string `json:"sender"`
	MessageID string `json:"message_id"`
	FromMe    bool   `json:"from_me"`
	Star      bool   `json:"star"`
}

// StarMessageResult is the response envelope for POST /message/star.
type StarMessageResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}
