package storage

// EnsureS3ClientForUser loads S3 config from DB and initializes client if not already present (lazy init for reconnect-after-restart)
func EnsureS3ClientForUser(userID string) {
	GetS3Manager().EnsureClientFromDB(userID)
}
