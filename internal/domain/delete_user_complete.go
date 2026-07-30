package domain

// DeleteUserCompleteRequest represents a request to completely delete a user
type DeleteUserCompleteRequest struct {
	ID string `param:"id"`
}

// DeleteUserCompleteResult represents the result of completely deleting a user
type DeleteUserCompleteResult struct {
	Code    int    `json:"code"`
	Data    UserDeleteData `json:"data"`
	Success bool   `json:"success"`
	Details string `json:"details"`
}

// UserDeleteData represents the user data returned after deletion
type UserDeleteData struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	JID  string `json:"jid"`
}
