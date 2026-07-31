package port

// DeleteUserProvider abstracts database and storage operations for user deletion
type DeleteUserProvider interface {
	// CheckUserExists checks if a user exists by ID
	CheckUserExists(userID string) (bool, error)
	// GetUserInfo retrieves user information
	GetUserInfo(userID string) (name, jid, token string, err error)
	// DeleteUserFromDB deletes a user from database
	DeleteUserFromDB(userID string) error
	// GetS3Enabled checks if S3 is enabled for a user
	GetS3Enabled(userID string) (bool, error)
	// DeleteLocalUserFiles deletes local files for a user
	DeleteLocalUserFiles(userID, exPath string) error
}
