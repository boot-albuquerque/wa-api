package domain

// RejectCallRequest represents a request to reject a call
type RejectCallRequest struct {
	CallFrom string `json:"call_from"`
	CallID   string `json:"call_id"`
}

// RejectCallResult represents the result of rejecting a call
type RejectCallResult struct {
	Details string `json:"Details"`
	CallID  string `json:"CallID"`
}
