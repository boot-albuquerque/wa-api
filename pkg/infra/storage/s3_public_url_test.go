package storage

import (
	"context"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// TestGetPublicURL_CustomPublicURL proves the config.PublicURL override
// path is untouched by the presigning change: when the user configured
// their own CDN/reverse-proxy URL, we use it as-is.
func TestGetPublicURL_CustomPublicURL(t *testing.T) {
	m := &S3Manager{clients: make(map[string]*s3.Client), configs: make(map[string]*S3Config)}
	err := m.InitializeS3Client("user1", &S3Config{
		Enabled:   true,
		Region:    "us-east-1",
		Bucket:    "mybucket",
		AccessKey: "fake",
		SecretKey: "fake",
		PublicURL: "https://cdn.example.com/",
	})
	if err != nil {
		t.Fatalf("InitializeS3Client: %v", err)
	}

	got, err := m.GetPublicURL(context.Background(), "user1", "users/user1/inbox/x/2024/01/01/images/msg.jpg")
	if err != nil {
		t.Fatalf("GetPublicURL: %v", err)
	}
	want := "https://cdn.example.com/mybucket/users/user1/inbox/x/2024/01/01/images/msg.jpg"
	if got != want {
		t.Errorf("GetPublicURL() = %q, want %q", got, want)
	}
}

// TestGetPublicURL_Presigned proves that, absent a custom PublicURL, the
// object is no longer served via a plain constructed URL (UploadToS3 no
// longer sets ACL: public-read — see sec/F26) but via a presigned
// GetObject URL. SigV4 presigning is a local signing operation with no
// network call, so this runs against fake credentials without hitting AWS.
func TestGetPublicURL_Presigned(t *testing.T) {
	m := &S3Manager{clients: make(map[string]*s3.Client), configs: make(map[string]*S3Config)}
	err := m.InitializeS3Client("user1", &S3Config{
		Enabled:   true,
		Region:    "us-east-1",
		Bucket:    "mybucket",
		AccessKey: "fake-access-key",
		SecretKey: "fake-secret-key",
	})
	if err != nil {
		t.Fatalf("InitializeS3Client: %v", err)
	}

	got, err := m.GetPublicURL(context.Background(), "user1", "users/user1/inbox/x/2024/01/01/images/msg.jpg")
	if err != nil {
		t.Fatalf("GetPublicURL: %v", err)
	}

	if !strings.Contains(got, "mybucket") {
		t.Errorf("presigned URL %q does not reference the bucket", got)
	}
	if !strings.Contains(got, "X-Amz-Signature=") {
		t.Errorf("presigned URL %q is missing the SigV4 signature — looks like a plain constructed URL, not a presigned one", got)
	}
	if !strings.Contains(got, "X-Amz-Expires=") {
		t.Errorf("presigned URL %q is missing the expiry parameter", got)
	}
}

func TestGetPublicURL_NoClient(t *testing.T) {
	m := &S3Manager{clients: make(map[string]*s3.Client), configs: make(map[string]*S3Config)}
	_, err := m.GetPublicURL(context.Background(), "no-such-user", "some/key")
	if err == nil {
		t.Fatal("expected error for user with no S3 client configured, got nil")
	}
}
