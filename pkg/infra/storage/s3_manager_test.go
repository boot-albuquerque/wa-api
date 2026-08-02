package storage

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

// fakeS3 is an in-process stand-in for an S3-compatible endpoint. The AWS SDK
// v2 client is a concrete struct with no injectable seam, but it does accept a
// BaseEndpoint — pointing it at an httptest server exercises the real request
// construction, signing and XML response parsing without any network or Docker
// dependency.
type fakeS3 struct {
	mu sync.Mutex

	// puts records the object keys received via PutObject.
	puts []string
	// deleted records every key passed to DeleteObjects.
	deleted []string
	// listCalls counts ListObjectsV2 requests.
	listCalls int

	// listPages, when non-empty, is consumed one entry per ListObjectsV2 call.
	listPages []listPage
	// failList / failPut / failDelete make the respective operation answer with
	// a non-retryable 403 so the error branch is reached on the first attempt.
	failList   bool
	failPut    bool
	failDelete bool

	// onPut, if set, runs after a successful PutObject is recorded. It is the
	// seam used to mutate manager state mid-operation.
	onPut func()
}

type listPage struct {
	keys      []string
	nextToken string
}

func (f *fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	switch {
	case r.Method == http.MethodPut:
		if f.failPut {
			writeS3Error(w)
			return
		}
		f.puts = append(f.puts, strings.TrimPrefix(r.URL.Path, "/"))
		if f.onPut != nil {
			f.onPut()
		}
		w.WriteHeader(http.StatusOK)

	case r.Method == http.MethodPost && r.URL.Query().Has("delete"):
		if f.failDelete {
			writeS3Error(w)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		f.deleted = append(f.deleted, parseDeleteKeys(string(body))...)
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><DeleteResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"></DeleteResult>`))

	case r.Method == http.MethodGet:
		if f.failList {
			writeS3Error(w)
			return
		}
		page := listPage{}
		if f.listCalls < len(f.listPages) {
			page = f.listPages[f.listCalls]
		}
		f.listCalls++
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(renderListXML(page)))

	default:
		w.WriteHeader(http.StatusNotImplemented)
	}
}

func writeS3Error(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/xml")
	// 403 is deliberately chosen over 500: the SDK's default retryer treats 5xx
	// as retryable, which would turn every error-branch assertion into three
	// round trips plus backoff sleeps.
	w.WriteHeader(http.StatusForbidden)
	_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><Error><Code>AccessDenied</Code><Message>denied</Message></Error>`))
}

func renderListXML(p listPage) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Name>bucket</Name>`)
	fmt.Fprintf(&b, `<KeyCount>%d</KeyCount><MaxKeys>1000</MaxKeys>`, len(p.keys))
	if p.nextToken != "" {
		fmt.Fprintf(&b, `<IsTruncated>true</IsTruncated><NextContinuationToken>%s</NextContinuationToken>`, p.nextToken)
	} else {
		b.WriteString(`<IsTruncated>false</IsTruncated>`)
	}
	for _, k := range p.keys {
		fmt.Fprintf(&b, `<Contents><Key>%s</Key><Size>1</Size></Contents>`, k)
	}
	b.WriteString(`</ListBucketResult>`)
	return b.String()
}

// parseDeleteKeys pulls the <Key>..</Key> values out of a DeleteObjects body
// without pulling in an XML schema for a three-element document.
func parseDeleteKeys(body string) []string {
	var keys []string
	rest := body
	for {
		i := strings.Index(rest, "<Key>")
		if i < 0 {
			return keys
		}
		rest = rest[i+len("<Key>"):]
		j := strings.Index(rest, "</Key>")
		if j < 0 {
			return keys
		}
		keys = append(keys, rest[:j])
		rest = rest[j:]
	}
}

// newManagerWithFake returns a manager whose "user1" client talks to srv.
func newManagerWithFake(t *testing.T, fake *fakeS3, retentionDays int) (*S3Manager, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)

	m := newManager()
	if err := m.InitializeS3Client("user1", &S3Config{
		Enabled:       true,
		Endpoint:      srv.URL,
		Region:        "us-east-1",
		Bucket:        "mybucket",
		AccessKey:     "ak",
		SecretKey:     "sk",
		PathStyle:     true,
		RetentionDays: retentionDays,
	}); err != nil {
		t.Fatalf("InitializeS3Client: %v", err)
	}
	return m, srv
}

func newManager() *S3Manager {
	return &S3Manager{
		clients: make(map[string]*s3.Client),
		configs: make(map[string]*S3Config),
	}
}

func TestGetS3Manager_ReturnsSingleton(t *testing.T) {
	if GetS3Manager() != GetS3Manager() {
		t.Fatal("GetS3Manager returned different instances")
	}
	if GetS3Manager() != s3Manager {
		t.Fatal("GetS3Manager did not return the package-level manager")
	}
}

func TestInitializeS3Client_DisabledRemovesClient(t *testing.T) {
	m := newManager()
	if err := m.InitializeS3Client("user1", &S3Config{Enabled: true, Region: "us-east-1", Bucket: "b"}); err != nil {
		t.Fatalf("InitializeS3Client: %v", err)
	}
	if _, _, ok := m.GetClient("user1"); !ok {
		t.Fatal("expected client after enabled init")
	}

	if err := m.InitializeS3Client("user1", &S3Config{Enabled: false}); err != nil {
		t.Fatalf("InitializeS3Client(disabled): %v", err)
	}
	if _, _, ok := m.GetClient("user1"); ok {
		t.Fatal("expected client to be removed when config is disabled")
	}
}

func TestRemoveClient_IsIdempotent(t *testing.T) {
	m := newManager()
	m.RemoveClient("ghost")
	if _, _, ok := m.GetClient("ghost"); ok {
		t.Fatal("removing an unknown user must not create state")
	}
}

func TestGetClient_ReturnsConfig(t *testing.T) {
	m := newManager()
	cfg := &S3Config{Enabled: true, Region: "us-east-1", Bucket: "mybucket", Endpoint: "https://example.invalid"}
	if err := m.InitializeS3Client("user1", cfg); err != nil {
		t.Fatalf("InitializeS3Client: %v", err)
	}
	client, got, ok := m.GetClient("user1")
	if !ok {
		t.Fatal("expected client to be registered")
	}
	if client == nil {
		t.Fatal("expected non-nil S3 client")
	}
	if got.Bucket != "mybucket" {
		t.Errorf("config bucket = %q, want %q", got.Bucket, "mybucket")
	}
}

func TestUploadToS3_Success(t *testing.T) {
	fake := &fakeS3{}
	m, _ := newManagerWithFake(t, fake, 30)

	key := "users/user1/inbox/x/2024/01/01/images/msg.jpg"
	if err := m.UploadToS3(context.Background(), "user1", key, []byte("payload"), "image/jpeg"); err != nil {
		t.Fatalf("UploadToS3: %v", err)
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.puts) != 1 || fake.puts[0] != "mybucket/"+key {
		t.Fatalf("unexpected PutObject targets: %v", fake.puts)
	}
}

// TestUploadToS3_DefaultContentType covers the empty-mimeType branch and the
// zero-retention branch (no Expires header) in one upload.
func TestUploadToS3_DefaultContentTypeAndNoRetention(t *testing.T) {
	fake := &fakeS3{}
	m, _ := newManagerWithFake(t, fake, 0)

	if err := m.UploadToS3(context.Background(), "user1", "k", []byte("x"), ""); err != nil {
		t.Fatalf("UploadToS3: %v", err)
	}
}

// TestUploadToS3_InlineDispositionTypes exercises each mime family that gets
// ContentDisposition: inline.
func TestUploadToS3_InlineDispositionTypes(t *testing.T) {
	for _, mime := range []string{"image/png", "video/mp4", "application/pdf", "application/zip"} {
		t.Run(mime, func(t *testing.T) {
			fake := &fakeS3{}
			m, _ := newManagerWithFake(t, fake, 7)
			if err := m.UploadToS3(context.Background(), "user1", "k", []byte("x"), mime); err != nil {
				t.Fatalf("UploadToS3(%s): %v", mime, err)
			}
		})
	}
}

func TestUploadToS3_NoClientAndNoDB(t *testing.T) {
	m := newManager()
	err := m.UploadToS3(context.Background(), "ghost", "k", []byte("x"), "image/png")
	if err == nil {
		t.Fatal("expected error when no client is registered and no DB is set")
	}
	if !strings.Contains(err.Error(), "not initialized") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestUploadToS3_PutObjectFails(t *testing.T) {
	fake := &fakeS3{failPut: true}
	m, _ := newManagerWithFake(t, fake, 1)

	err := m.UploadToS3(context.Background(), "user1", "k", []byte("x"), "image/png")
	if err == nil {
		t.Fatal("expected upload error from the S3 endpoint")
	}
	if !strings.Contains(err.Error(), "failed to upload to S3") {
		t.Errorf("error not wrapped by UploadToS3: %v", err)
	}
}

func TestTestConnection_Success(t *testing.T) {
	fake := &fakeS3{listPages: []listPage{{keys: []string{"a"}}}}
	m, _ := newManagerWithFake(t, fake, 0)

	if err := m.TestConnection(context.Background(), "user1"); err != nil {
		t.Fatalf("TestConnection: %v", err)
	}
}

func TestTestConnection_ListFails(t *testing.T) {
	fake := &fakeS3{failList: true}
	m, _ := newManagerWithFake(t, fake, 0)

	if err := m.TestConnection(context.Background(), "user1"); err == nil {
		t.Fatal("expected error when the endpoint rejects ListObjectsV2")
	}
}

func TestTestConnection_NoClient(t *testing.T) {
	m := newManager()
	if err := m.TestConnection(context.Background(), "ghost"); err == nil {
		t.Fatal("expected error for unregistered user")
	}
}

func TestProcessMediaForS3_Success(t *testing.T) {
	fake := &fakeS3{}
	m, _ := newManagerWithFake(t, fake, 30)

	got, err := m.ProcessMediaForS3(context.Background(), "user1", "5511@s.whatsapp.net", "msg1",
		[]byte("hello"), "image/png", "photo.png", true)
	if err != nil {
		t.Fatalf("ProcessMediaForS3: %v", err)
	}

	if got["bucket"] != "mybucket" {
		t.Errorf("bucket = %v, want mybucket", got["bucket"])
	}
	if got["size"] != 5 {
		t.Errorf("size = %v, want 5", got["size"])
	}
	if got["mimeType"] != "image/png" {
		t.Errorf("mimeType = %v", got["mimeType"])
	}
	if got["fileName"] != "photo.png" {
		t.Errorf("fileName = %v", got["fileName"])
	}
	key, _ := got["key"].(string)
	if !strings.HasPrefix(key, "users/user1/inbox/") || !strings.HasSuffix(key, "/msg1.png") {
		t.Errorf("unexpected key %q", key)
	}
	url, _ := got["url"].(string)
	if !strings.Contains(url, "X-Amz-Signature=") {
		t.Errorf("expected a presigned URL, got %q", url)
	}
}

func TestProcessMediaForS3_UploadFails(t *testing.T) {
	fake := &fakeS3{failPut: true}
	m, _ := newManagerWithFake(t, fake, 0)

	_, err := m.ProcessMediaForS3(context.Background(), "user1", "5511@s.whatsapp.net", "msg1",
		[]byte("hello"), "image/png", "photo.png", false)
	if err == nil {
		t.Fatal("expected error when upload fails")
	}
	if !strings.Contains(err.Error(), "failed to upload to S3") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestProcessMediaForS3_URLFails removes the client between upload and URL
// generation is not reachable through the public API, so instead the whole
// operation is run for an unregistered user: UploadToS3 fails first, which is
// the only ordering the code allows. The URL-failure branch is covered by
// GetPublicURL's own tests.
func TestProcessMediaForS3_NoClient(t *testing.T) {
	m := newManager()
	_, err := m.ProcessMediaForS3(context.Background(), "ghost", "5511@s.whatsapp.net", "msg1",
		[]byte("hello"), "image/png", "photo.png", false)
	if err == nil {
		t.Fatal("expected error for unregistered user")
	}
}

// TestProcessMediaForS3_URLGenerationFails reaches the branch between the two
// remote calls: the upload succeeds, then the user's S3 client disappears
// before the URL is built. That is a real ordering — a user disabling S3 (or a
// reconfigure) racing an in-flight upload — reproduced deterministically by
// removing the client from inside the PutObject handler.
func TestProcessMediaForS3_URLGenerationFails(t *testing.T) {
	fake := &fakeS3{}
	m, _ := newManagerWithFake(t, fake, 0)

	fake.mu.Lock()
	fake.onPut = func() { m.RemoveClient("user1") }
	fake.mu.Unlock()

	_, err := m.ProcessMediaForS3(context.Background(), "user1", "5511@s.whatsapp.net", "msg1",
		[]byte("hello"), "image/png", "photo.png", true)
	if err == nil {
		t.Fatal("expected error when the client vanishes before URL generation")
	}
	if !strings.Contains(err.Error(), "failed to generate S3 URL") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDeleteAllUserObjects_NoClient(t *testing.T) {
	m := newManager()
	if err := m.DeleteAllUserObjects(context.Background(), "ghost"); err == nil {
		t.Fatal("expected error for unregistered user")
	}
}

func TestDeleteAllUserObjects_SinglePage(t *testing.T) {
	fake := &fakeS3{listPages: []listPage{{keys: []string{"users/user1/a", "users/user1/b"}}}}
	m, _ := newManagerWithFake(t, fake, 0)

	if err := m.DeleteAllUserObjects(context.Background(), "user1"); err != nil {
		t.Fatalf("DeleteAllUserObjects: %v", err)
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.deleted) != 2 {
		t.Fatalf("deleted %d keys, want 2 (%v)", len(fake.deleted), fake.deleted)
	}
}

func TestDeleteAllUserObjects_EmptyBucketDeletesNothing(t *testing.T) {
	fake := &fakeS3{listPages: []listPage{{}}}
	m, _ := newManagerWithFake(t, fake, 0)

	if err := m.DeleteAllUserObjects(context.Background(), "user1"); err != nil {
		t.Fatalf("DeleteAllUserObjects: %v", err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.deleted) != 0 {
		t.Fatalf("expected no DeleteObjects call, got %v", fake.deleted)
	}
}

// TestDeleteAllUserObjects_PaginatesAndBatches drives the two loop branches
// that only fire at scale: the continuation-token follow-up request, and the
// mid-page flush once the pending batch reaches S3's 1000-key limit.
func TestDeleteAllUserObjects_PaginatesAndBatches(t *testing.T) {
	first := make([]string, 900)
	for i := range first {
		first[i] = fmt.Sprintf("users/user1/p1-%04d", i)
	}
	second := make([]string, 300)
	for i := range second {
		second[i] = fmt.Sprintf("users/user1/p2-%04d", i)
	}

	fake := &fakeS3{listPages: []listPage{
		{keys: first, nextToken: "tok"},
		{keys: second},
	}}
	m, _ := newManagerWithFake(t, fake, 0)

	if err := m.DeleteAllUserObjects(context.Background(), "user1"); err != nil {
		t.Fatalf("DeleteAllUserObjects: %v", err)
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.listCalls != 2 {
		t.Errorf("listCalls = %d, want 2 (continuation token not followed)", fake.listCalls)
	}
	if len(fake.deleted) != 1200 {
		t.Errorf("deleted %d keys, want 1200", len(fake.deleted))
	}
}

func TestDeleteAllUserObjects_ListFails(t *testing.T) {
	fake := &fakeS3{failList: true}
	m, _ := newManagerWithFake(t, fake, 0)

	err := m.DeleteAllUserObjects(context.Background(), "user1")
	if err == nil {
		t.Fatal("expected error when listing fails")
	}
	if !strings.Contains(err.Error(), "failed to list objects") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDeleteAllUserObjects_FinalDeleteFails(t *testing.T) {
	fake := &fakeS3{
		listPages:  []listPage{{keys: []string{"users/user1/a"}}},
		failDelete: true,
	}
	m, _ := newManagerWithFake(t, fake, 0)

	err := m.DeleteAllUserObjects(context.Background(), "user1")
	if err == nil {
		t.Fatal("expected error when the trailing DeleteObjects fails")
	}
	if !strings.Contains(err.Error(), "failed to delete objects") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestDeleteAllUserObjects_BatchDeleteFails hits the in-loop DeleteObjects
// error branch, which is only reachable once a page carries 1000 keys.
func TestDeleteAllUserObjects_BatchDeleteFails(t *testing.T) {
	keys := make([]string, 1000)
	for i := range keys {
		keys[i] = fmt.Sprintf("users/user1/k-%04d", i)
	}
	fake := &fakeS3{
		listPages:  []listPage{{keys: keys}},
		failDelete: true,
	}
	m, _ := newManagerWithFake(t, fake, 0)

	err := m.DeleteAllUserObjects(context.Background(), "user1")
	if err == nil {
		t.Fatal("expected error when the batched DeleteObjects fails")
	}
	if !strings.Contains(err.Error(), "failed to delete objects") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGetPublicURL_PresignFailsOnBadCredentials(t *testing.T) {
	// An empty secret key makes SigV4 signing itself fail, which is the only
	// way to reach the presign error branch without a network call.
	m := newManager()
	if err := m.InitializeS3Client("user1", &S3Config{
		Enabled: true,
		Region:  "us-east-1",
		Bucket:  "mybucket",
	}); err != nil {
		t.Fatalf("InitializeS3Client: %v", err)
	}
	if _, err := m.GetPublicURL(context.Background(), "user1", "some/key"); err == nil {
		t.Fatal("expected presign to fail without credentials")
	}
}

// --- lazy init from the database -------------------------------------------

func openStorageTestDB(t *testing.T) *sqlx.DB {
	t.Helper()
	db, err := sqlx.Open("sqlite", filepath.Join(t.TempDir(), "storage.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close db: %v", err)
		}
	})
	schema := `CREATE TABLE users (
		id TEXT PRIMARY KEY,
		s3_enabled BOOLEAN NOT NULL DEFAULT 0,
		s3_endpoint TEXT NOT NULL DEFAULT '',
		s3_region TEXT NOT NULL DEFAULT '',
		s3_bucket TEXT NOT NULL DEFAULT '',
		s3_access_key TEXT NOT NULL DEFAULT '',
		s3_secret_key TEXT NOT NULL DEFAULT '',
		s3_path_style BOOLEAN NOT NULL DEFAULT 0,
		s3_public_url TEXT NOT NULL DEFAULT '',
		media_delivery TEXT,
		s3_retention_days INTEGER
	)`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("create users table: %v", err)
	}
	return db
}

func insertUser(t *testing.T, db *sqlx.DB, id string, enabled bool) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO users (id, s3_enabled, s3_endpoint, s3_region, s3_bucket, s3_access_key, s3_secret_key, s3_path_style, s3_public_url, media_delivery, s3_retention_days)
		 VALUES (?, ?, 'https://s3.example.invalid', 'us-east-1', 'user-bucket', 'ak', 'sk', 1, '', NULL, NULL)`,
		id, enabled)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
}

func TestEnsureClientFromDB_NoDB(t *testing.T) {
	m := newManager()
	if m.EnsureClientFromDB("user1") {
		t.Fatal("expected false when no database reference is set")
	}
}

func TestEnsureClientFromDB_AlreadyInitialized(t *testing.T) {
	m := newManager()
	if err := m.InitializeS3Client("user1", &S3Config{Enabled: true, Region: "us-east-1", Bucket: "b"}); err != nil {
		t.Fatalf("InitializeS3Client: %v", err)
	}
	// No DB is set: returning true proves the short-circuit on the existing
	// client ran before the DB lookup.
	if !m.EnsureClientFromDB("user1") {
		t.Fatal("expected true for an already-initialized client")
	}
}

func TestEnsureClientFromDB_QueryError(t *testing.T) {
	db := openStorageTestDB(t)
	m := newManager()
	m.SetDB(db)

	if m.EnsureClientFromDB("missing-user") {
		t.Fatal("expected false when the user row does not exist")
	}
	if _, _, ok := m.GetClient("missing-user"); ok {
		t.Fatal("no client should have been registered")
	}
}

func TestEnsureClientFromDB_DisabledUser(t *testing.T) {
	db := openStorageTestDB(t)
	insertUser(t, db, "user1", false)

	m := newManager()
	m.SetDB(db)

	if m.EnsureClientFromDB("user1") {
		t.Fatal("expected false when S3 is disabled for the user")
	}
}

func TestEnsureClientFromDB_InitializesFromRow(t *testing.T) {
	db := openStorageTestDB(t)
	insertUser(t, db, "user1", true)

	m := newManager()
	m.SetDB(db)

	if !m.EnsureClientFromDB("user1") {
		t.Fatal("expected the client to be lazily initialized from the users row")
	}
	_, cfg, ok := m.GetClient("user1")
	if !ok {
		t.Fatal("client not registered after lazy init")
	}
	if cfg.Bucket != "user-bucket" {
		t.Errorf("bucket = %q, want user-bucket", cfg.Bucket)
	}
	if !cfg.PathStyle {
		t.Error("PathStyle not carried over from the row")
	}
	// COALESCE defaults on the two nullable columns.
	if cfg.MediaDelivery != "base64" {
		t.Errorf("MediaDelivery = %q, want base64 (COALESCE default)", cfg.MediaDelivery)
	}
	if cfg.RetentionDays != 30 {
		t.Errorf("RetentionDays = %d, want 30 (COALESCE default)", cfg.RetentionDays)
	}
}

// TestUploadToS3_LazyInitFromDB proves the reconnect-after-restart path:
// UploadToS3 is called for a user with no in-memory client, and recovers it
// from the database before uploading.
func TestUploadToS3_LazyInitFromDB(t *testing.T) {
	fake := &fakeS3{}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)

	db := openStorageTestDB(t)
	_, err := db.Exec(
		`INSERT INTO users (id, s3_enabled, s3_endpoint, s3_region, s3_bucket, s3_access_key, s3_secret_key, s3_path_style, s3_public_url, media_delivery, s3_retention_days)
		 VALUES ('user1', 1, ?, 'us-east-1', 'mybucket', 'ak', 'sk', 1, '', 's3', 5)`, srv.URL)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}

	m := newManager()
	m.SetDB(db)

	if err := m.UploadToS3(context.Background(), "user1", "k", []byte("x"), "image/png"); err != nil {
		t.Fatalf("UploadToS3 with lazy init: %v", err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.puts) != 1 {
		t.Fatalf("expected exactly one PutObject, got %v", fake.puts)
	}
}

// TestEnsureS3ClientForUser exercises the package-level helper, which routes
// through the global manager.
func TestEnsureS3ClientForUser(t *testing.T) {
	prev := GetS3Manager()
	prev.mu.Lock()
	prevDB := prev.db
	prev.mu.Unlock()
	t.Cleanup(func() {
		prev.mu.Lock()
		prev.db = prevDB
		prev.mu.Unlock()
		prev.RemoveClient("global-user")
	})

	db := openStorageTestDB(t)
	insertUser(t, db, "global-user", true)
	prev.SetDB(db)

	EnsureS3ClientForUser("global-user")

	if _, _, ok := prev.GetClient("global-user"); !ok {
		t.Fatal("EnsureS3ClientForUser did not initialize the client on the global manager")
	}
}
