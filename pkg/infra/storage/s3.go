package storage

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"
)

// S3Config holds S3 configuration for a user
type S3Config struct {
	Enabled       bool
	Endpoint      string
	Region        string
	Bucket        string
	AccessKey     string
	SecretKey     string
	PathStyle     bool
	PublicURL     string
	MediaDelivery string
	RetentionDays int
}

// S3Manager manages S3 operations
type S3Manager struct {
	mu      sync.RWMutex
	db      *sqlx.DB
	clients map[string]*s3.Client
	configs map[string]*S3Config
}

// Global S3 manager instance
var s3Manager = &S3Manager{
	clients: make(map[string]*s3.Client),
	configs: make(map[string]*S3Config),
}

// GetS3Manager returns the global S3 manager instance
func GetS3Manager() *S3Manager {
	return s3Manager
}

// SetDB sets the database reference for lazy S3 client initialization
func (m *S3Manager) SetDB(db *sqlx.DB) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.db = db
	log.Debug().Bool("hasDB", db != nil).Msg("S3 manager database reference set")
}

// EnsureClientFromDB loads S3 config from DB and initializes client if enabled. Returns true if client is available.
func (m *S3Manager) EnsureClientFromDB(userID string) bool {
	if _, _, ok := m.GetClient(userID); ok {
		return true
	}
	m.mu.RLock()
	db := m.db
	m.mu.RUnlock()
	if db == nil {
		log.Warn().Str("userID", userID).Msg("S3 lazy init skipped: no database reference on manager")
		return false
	}
	var s3DbConfig struct {
		Enabled       bool   `db:"s3_enabled"`
		Endpoint      string `db:"s3_endpoint"`
		Region        string `db:"s3_region"`
		Bucket        string `db:"s3_bucket"`
		AccessKey     string `db:"s3_access_key"`
		SecretKey     string `db:"s3_secret_key"`
		PathStyle     bool   `db:"s3_path_style"`
		PublicURL     string `db:"s3_public_url"`
		MediaDelivery string `db:"media_delivery"`
		RetentionDays int    `db:"s3_retention_days"`
	}
	query := `SELECT s3_enabled, s3_endpoint, s3_region, s3_bucket, s3_access_key, s3_secret_key, s3_path_style, s3_public_url, COALESCE(media_delivery, 'base64') AS media_delivery, COALESCE(s3_retention_days, 30) AS s3_retention_days FROM users WHERE id = $1`
	query = db.Rebind(query)
	if err := db.Get(&s3DbConfig, query, userID); err != nil {
		log.Warn().Err(err).Str("userID", userID).Msg("failed to load S3 config from database, lazy init aborted")
		return false
	}
	if !s3DbConfig.Enabled {
		return false
	}
	config := &S3Config{
		Enabled:       s3DbConfig.Enabled,
		Endpoint:      s3DbConfig.Endpoint,
		Region:        s3DbConfig.Region,
		Bucket:        s3DbConfig.Bucket,
		AccessKey:     s3DbConfig.AccessKey,
		SecretKey:     s3DbConfig.SecretKey,
		PathStyle:     s3DbConfig.PathStyle,
		PublicURL:     s3DbConfig.PublicURL,
		MediaDelivery: s3DbConfig.MediaDelivery,
		RetentionDays: s3DbConfig.RetentionDays,
	}
	return m.InitializeS3Client(userID, config) == nil
}

// InitializeS3Client creates or updates S3 client for a user
func (m *S3Manager) InitializeS3Client(userID string, config *S3Config) error {
	if !config.Enabled {
		m.RemoveClient(userID)
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Create custom credentials provider
	credProvider := credentials.NewStaticCredentialsProvider(
		config.AccessKey,
		config.SecretKey,
		"",
	)

	// Configure S3 client
	cfg := aws.Config{
		Region:      config.Region,
		Credentials: credProvider,
	}

	// Create S3 client
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.UsePathStyle = config.PathStyle
		if config.Endpoint != "" {
			o.BaseEndpoint = aws.String(config.Endpoint)
		}
	})

	m.clients[userID] = client
	m.configs[userID] = config

	log.Info().Str("userID", userID).Str("bucket", config.Bucket).Msg("S3 client initialized")
	return nil
}

// RemoveClient removes S3 client for a user
func (m *S3Manager) RemoveClient(userID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.clients, userID)
	delete(m.configs, userID)

	log.Debug().Str("userID", userID).Msg("S3 client removed")
}

// GetClient returns S3 client for a user
func (m *S3Manager) GetClient(userID string) (*s3.Client, *S3Config, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	client, clientOk := m.clients[userID]
	config, configOk := m.configs[userID]

	if !clientOk || !configOk {
		log.Debug().Str("userID", userID).Msg("no S3 client registered for user")
	}

	return client, config, clientOk && configOk
}

// s3KeyComponentPattern matches any character not safe inside a single S3
// key path segment. Deliberately conservative (allow-list, not deny-list):
// anything outside [A-Za-z0-9_-] becomes "_". "." is excluded on purpose,
// not just "/" — replacing "/" alone in "../../../etc/passwd" leaves
// ".." sequences sitting mid-string ("_.._.._etc_passwd"), so excluding
// "." entirely is what actually guarantees no ".." can ever reach the
// interpolated key. A WhatsApp message ID has no legitimate need for a
// literal dot.
var s3KeyComponentPattern = regexp.MustCompile(`[^A-Za-z0-9_-]`)

// sanitizeS3KeyComponent makes s safe to interpolate as one segment of an
// S3 key: neither "/" (extra path segments) nor "." (which combined with
// "/" could form "..") can survive.
func sanitizeS3KeyComponent(s string) string {
	return s3KeyComponentPattern.ReplaceAllString(s, "_")
}

// GenerateS3Key generates S3 object key based on message metadata
func (m *S3Manager) GenerateS3Key(userID, contactJID, messageID string, mimeType string, isIncoming bool) string {
	return m.generateS3KeyAt(time.Now(), userID, contactJID, messageID, mimeType, isIncoming)
}

// generateS3KeyAt is GenerateS3Key with the timestamp injected, so the date
// partitioning can be tested deterministically.
func (m *S3Manager) generateS3KeyAt(now time.Time, userID, contactJID, messageID string, mimeType string, isIncoming bool) string {
	// Determine direction
	direction := "outbox"
	if isIncoming {
		direction = "inbox"
	}

	// Clean contact JID
	contactJID = strings.ReplaceAll(contactJID, "@", "_")
	contactJID = strings.ReplaceAll(contactJID, ":", "_")

	// messageID comes from whatsmeow's MessageInfo.ID, a raw string attribute
	// off the incoming <message> stanza (verified against whatsmeow's own
	// source: types/jid.go's MessageID is a plain string alias, and
	// message.go's parseMessageInfo sets it from ag.String("id") with no
	// format validation). For an incoming message, that attribute is set by
	// the sender, not this SDK — an unsanitized "/" or "../" here would
	// traverse the direction/date prefixes above and let one partition
	// collide with or overwrite another (sec/F28, confirmed in Fase 1-v).
	messageID = sanitizeS3KeyComponent(messageID)

	year := now.Format("2006")
	month := now.Format("01")
	day := now.Format("02")

	// Determine media type folder
	mediaType := "documents"
	if strings.HasPrefix(mimeType, "image/") {
		mediaType = "images"
	} else if strings.HasPrefix(mimeType, "video/") {
		mediaType = "videos"
	} else if strings.HasPrefix(mimeType, "audio/") {
		mediaType = "audio"
	}

	// Get file extension
	ext := ".bin"
	switch {
	case strings.Contains(mimeType, "jpeg"), strings.Contains(mimeType, "jpg"):
		ext = ".jpg"
	case strings.Contains(mimeType, "png"):
		ext = ".png"
	case strings.Contains(mimeType, "gif"):
		ext = ".gif"
	case strings.Contains(mimeType, "webp"):
		ext = ".webp"
	case strings.Contains(mimeType, "mp4"):
		ext = ".mp4"
	case strings.Contains(mimeType, "webm"):
		ext = ".webm"
	case strings.Contains(mimeType, "ogg"):
		ext = ".ogg"
	case strings.Contains(mimeType, "opus"):
		ext = ".opus"
	case strings.Contains(mimeType, "pdf"):
		ext = ".pdf"
	case strings.Contains(mimeType, "spreadsheetml"):
		ext = ".xlsx"
	case strings.Contains(mimeType, "excel"):
		ext = ".xls"
	case strings.Contains(mimeType, "doc"):
		if strings.Contains(mimeType, "docx") {
			ext = ".docx"
		} else {
			ext = ".doc"
		}
	}

	// Build S3 key
	key := fmt.Sprintf("users/%s/%s/%s/%s/%s/%s/%s/%s%s",
		userID,
		direction,
		contactJID,
		year,
		month,
		day,
		mediaType,
		messageID,
		ext,
	)

	log.Debug().Str("userID", userID).Str("key", key).Str("mimeType", mimeType).Msg("S3 object key generated")

	return key
}

// UploadToS3 uploads file to S3 and returns the key
func (m *S3Manager) UploadToS3(ctx context.Context, userID string, key string, data []byte, mimeType string) error {
	client, config, ok := m.GetClient(userID)
	if !ok {
		// Try lazy init from DB if available (handles reconnect-after-restart)
		if m.EnsureClientFromDB(userID) {
			client, config, ok = m.GetClient(userID)
		}
		if !ok {
			log.Error().Str("userID", userID).Str("key", key).Msg("S3 upload aborted: client not initialized for user")
			return fmt.Errorf("S3 client not initialized for user %s", userID)
		}
	}

	// Set content type and cache headers for preview
	contentType := mimeType
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// Calculate expiration time based on retention days
	var expires *time.Time
	if config.RetentionDays > 0 {
		expirationTime := time.Now().Add(time.Duration(config.RetentionDays) * 24 * time.Hour)
		expires = &expirationTime
	}

	input := &s3.PutObjectInput{
		Bucket:       aws.String(config.Bucket),
		Key:          aws.String(key),
		Body:         bytes.NewReader(data),
		ContentType:  aws.String(contentType),
		CacheControl: aws.String("public, max-age=3600"),
	}

	if expires != nil {
		input.Expires = expires
	}

	// Add content disposition for inline preview
	if strings.HasPrefix(mimeType, "image/") || strings.HasPrefix(mimeType, "video/") || mimeType == "application/pdf" {
		input.ContentDisposition = aws.String("inline")
	}

	_, err := client.PutObject(ctx, input)
	if err != nil {
		log.Error().Err(err).Str("userID", userID).Str("bucket", config.Bucket).Str("key", key).Msg("failed to upload object to S3")
		return fmt.Errorf("failed to upload to S3: %w", err)
	}

	return nil
}

// presignExpiry is how long a presigned GetObject URL stays valid. 7 days
// is the maximum SigV4 allows; media links in a chat app are viewed well
// within that window, and re-presigning is just calling this again.
const presignExpiry = 7 * 24 * time.Hour

// GetPublicURL returns a URL the object can be fetched from without AWS
// credentials. UploadToS3 no longer sets ACL: public-read (Fase 1b removed
// it; Block Public Access was not confirmed enabled — Fase 1-v, sec/F26),
// so a plain constructed URL would 403 for anyone who isn't the bucket
// owner. A presigned GetObject URL is the replacement: it grants
// time-limited access to that one object without making the object or the
// bucket public. If config.PublicURL is set (the user pointed a CDN or
// reverse proxy at the bucket themselves), that's used as-is — presigning
// is only for the case where this process's own credentials are what
// grant access.
func (m *S3Manager) GetPublicURL(ctx context.Context, userID, key string) (string, error) {
	client, config, ok := m.GetClient(userID)
	if !ok {
		log.Error().Str("userID", userID).Str("key", key).Msg("cannot build S3 URL: client not initialized for user")
		return "", fmt.Errorf("S3 client not initialized for user %s", userID)
	}

	if config.PublicURL != "" {
		return fmt.Sprintf("%s/%s/%s", strings.TrimRight(config.PublicURL, "/"), config.Bucket, key), nil
	}

	presignClient := s3.NewPresignClient(client)
	req, err := presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(config.Bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(presignExpiry))
	if err != nil {
		log.Error().Err(err).Str("userID", userID).Str("bucket", config.Bucket).Str("key", key).Msg("failed to presign S3 GetObject URL")
		return "", fmt.Errorf("failed to presign S3 URL: %w", err)
	}
	return req.URL, nil
}

// TestConnection tests S3 connection
func (m *S3Manager) TestConnection(ctx context.Context, userID string) error {
	client, config, ok := m.GetClient(userID)
	if !ok {
		log.Error().Str("userID", userID).Msg("S3 connection test aborted: client not initialized for user")
		return fmt.Errorf("S3 client not initialized for user %s", userID)
	}

	// Try to list objects with max 1 result
	input := &s3.ListObjectsV2Input{
		Bucket:  aws.String(config.Bucket),
		MaxKeys: aws.Int32(1),
	}

	_, err := client.ListObjectsV2(ctx, input)
	return err
}

// ProcessMediaForS3 handles the complete media upload process
func (m *S3Manager) ProcessMediaForS3(ctx context.Context, userID, contactJID, messageID string,
	data []byte, mimeType string, fileName string, isIncoming bool) (map[string]interface{}, error) {

	// Generate S3 key
	key := m.GenerateS3Key(userID, contactJID, messageID, mimeType, isIncoming)

	// Upload to S3
	err := m.UploadToS3(ctx, userID, key, data, mimeType)
	if err != nil {
		log.Error().Err(err).Str("userID", userID).Str("key", key).Str("fileName", fileName).Msg("media processing failed at S3 upload")
		return nil, fmt.Errorf("failed to upload to S3: %w", err)
	}

	// Generate public URL
	publicURL, err := m.GetPublicURL(ctx, userID, key)
	if err != nil {
		log.Error().Err(err).Str("userID", userID).Str("key", key).Str("fileName", fileName).Msg("media processing failed at S3 URL generation")
		return nil, fmt.Errorf("failed to generate S3 URL: %w", err)
	}

	// Read the bucket through GetClient, which acquires the read lock — this
	// avoids racing with a concurrent reconfigure/removal of the configs map and
	// the nil deref the previous unlocked read could hit.
	bucket := ""
	if _, config, ok := m.GetClient(userID); ok && config != nil {
		bucket = config.Bucket
	}

	// Return S3 metadata
	s3Data := map[string]interface{}{
		"url":      publicURL,
		"key":      key,
		"bucket":   bucket,
		"size":     len(data),
		"mimeType": mimeType,
		"fileName": fileName,
	}

	return s3Data, nil
}

// DeleteAllUserObjects deletes all user files from S3
func (m *S3Manager) DeleteAllUserObjects(ctx context.Context, userID string) error {
	client, config, ok := m.GetClient(userID)
	if !ok {
		log.Error().Str("userID", userID).Msg("S3 purge aborted: client not initialized for user")
		return fmt.Errorf("S3 client not initialized for user %s", userID)
	}

	prefix := fmt.Sprintf("users/%s/", userID)
	var toDelete []types.ObjectIdentifier
	var continuationToken *string

	for {
		input := &s3.ListObjectsV2Input{
			Bucket:            aws.String(config.Bucket),
			Prefix:            aws.String(prefix),
			ContinuationToken: continuationToken,
		}
		output, err := client.ListObjectsV2(ctx, input)
		if err != nil {
			return fmt.Errorf("failed to list objects for user %s: %w", userID, err)
		}

		for _, obj := range output.Contents {
			toDelete = append(toDelete, types.ObjectIdentifier{Key: obj.Key})
			// Delete in batches of 1000 (S3 limit)
			if len(toDelete) == 1000 {
				_, err := client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
					Bucket: aws.String(config.Bucket),
					Delete: &types.Delete{Objects: toDelete},
				})
				if err != nil {
					return fmt.Errorf("failed to delete objects for user %s: %w", userID, err)
				}
				toDelete = nil
			}
		}

		if output.IsTruncated != nil && *output.IsTruncated && output.NextContinuationToken != nil {
			continuationToken = output.NextContinuationToken
		} else {
			break
		}
	}

	// Delete any remaining objects
	if len(toDelete) > 0 {
		_, err := client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
			Bucket: aws.String(config.Bucket),
			Delete: &types.Delete{Objects: toDelete},
		})
		if err != nil {
			return fmt.Errorf("failed to delete objects for user %s: %w", userID, err)
		}
	}

	log.Info().Str("userID", userID).Msg("all user files removed from S3")
	return nil
}
