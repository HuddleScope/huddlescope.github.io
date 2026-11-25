package turso

import (
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/storage/sqlite"
	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
	_ "github.com/tursodatabase/libsql-client-go/libsql"
)

func TestBuildDSN(t *testing.T) {
	tests := []struct {
		name        string
		databaseURL string
		authToken   string
		want        string
		wantErr     bool
	}{
		{
			name:        "happy path with separate token",
			databaseURL: "libsql://test.turso.io",
			authToken:   "test-token-123",
			want:        "libsql://test.turso.io?authToken=test-token-123",
			wantErr:     false,
		},
		{
			name:        "token in userinfo (moved to query)",
			databaseURL: "libsql://token-xyz@test.turso.io",
			authToken:   "",
			want:        "libsql://test.turso.io?authToken=token-xyz",
			wantErr:     false,
		},
		{
			name:        "token in query (kept)",
			databaseURL: "libsql://test.turso.io?authToken=query-token",
			authToken:   "",
			want:        "libsql://test.turso.io?authToken=query-token",
			wantErr:     false,
		},
		{
			name:        "embedded token wins",
			databaseURL: "libsql://user@token@host/path?authToken=query",
			authToken:   "other",
			want:        "libsql://host/path?authToken=user%40token",
			wantErr:     false,
		},
		{
			name:        "token in query + separate (query wins)",
			databaseURL: "libsql://test.turso.io?authToken=query",
			authToken:   "separate",
			want:        "libsql://test.turso.io?authToken=query",
			wantErr:     false,
		},
		{
			name:        "invalid URL",
			databaseURL: "not-a-url",
			authToken:   "token",
			want:        "not-a-url?authToken=token",
			wantErr:     false,
		},
		{
			name:        "empty URL with token",
			databaseURL: "",
			authToken:   "token",
			want:        "?authToken=token",
			wantErr:     false,
		},
		{
			name:        "empty token is okay",
			databaseURL: "libsql://test.turso.io",
			authToken:   "",
			want:        "libsql://test.turso.io",
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildDSN(tt.databaseURL, tt.authToken)
			if (err != nil) != tt.wantErr {
				t.Errorf("buildDSN() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				if tt.authToken != "" && strings.Contains(err.Error(), tt.authToken) {
					t.Errorf("error contains token: %v", err)
				}
				return
			}
			if got != tt.want {
				t.Errorf("buildDSN() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRedactedDSN(t *testing.T) {
	tests := []struct {
		name string
		dsn  string
		want string
	}{
		{
			name: "authToken in query",
			dsn:  "libsql://host?authToken=secret123",
			want: "libsql://host?authToken=***",
		},
		{
			name: "auth_token in query",
			dsn:  "libsql://host?auth_token=secret123",
			want: "libsql://host?auth_token=***",
		},
		{
			name: "jwt in query",
			dsn:  "libsql://host?jwt=secret123",
			want: "libsql://host?jwt=***",
		},
		{
			name: "multiple params",
			dsn:  "libsql://host?authToken=sec&tls=1",
			want: "libsql://host?authToken=***&tls=1",
		},
		{
			name: "no secret",
			dsn:  "libsql://host?tls=1",
			want: "libsql://host?tls=1",
		},
		{
			name: "no query",
			dsn:  "libsql://host",
			want: "libsql://host",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RedactedDSN(tt.dsn)
			if got != tt.want {
				t.Errorf("RedactedDSN() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestOpen_BadCredentials(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("unauthorized"))
	}))
	defer ts.Close()

	u, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatal(err)
	}

	dsn := "libsql://" + u.Host + "/fake-db"
	_, err = Open(dsn, "wrong-token")
	if err == nil {
		t.Fatal("expected error for bad credentials")
	}
	if strings.Contains(err.Error(), "wrong-token") {
		t.Errorf("error leaks token: %v", err)
	}
	if err == nil || (!strings.Contains(err.Error(), "authentication") && !strings.Contains(err.Error(), "Post")) {
		t.Errorf("error should mention auth failure or HTTP request: %v", err)
	}
}

func TestOpen_FileDSN(t *testing.T) {
	tmpfile := t.TempDir() + "/test.db"
	dsn := "file:" + tmpfile

	db, err := Open(dsn, "")
	if err != nil {
		t.Fatalf("Open(file:...) failed: %v", err)
	}
	defer func() { _ = db.Close() }()

	var one int
	if err := db.QueryRow("SELECT 1").Scan(&one); err != nil {
		t.Fatalf("SELECT 1 failed: %v", err)
	}
	if one != 1 {
		t.Errorf("expected 1, got %d", one)
	}
}

func TestOpen_CredentialsRequired(t *testing.T) {
	_, err := Open("", "")
	if err == nil {
		t.Fatal("expected error for empty URL")
	}
	var ce *ConfigError
	if !errors.As(err, &ce) {
		t.Fatalf("expected *ConfigError, got %T", err)
	}
	if !strings.Contains(ce.Error(), "TURSO_DATABASE_URL") {
		t.Errorf("error should mention TURSO_DATABASE_URL: %v", ce)
	}
}

func TestUpsert_List_Count(t *testing.T) {
	tmpfile := t.TempDir() + "/repo-test.db"

	db, err := sql.Open("sqlite", "file:"+tmpfile+"?cache=shared")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	defer func() { _ = db.Close() }()

	if _, err := sqlite.MigrateDB(db); err != nil {
		t.Fatalf("MigrateDB failed: %v", err)
	}

	story := domain.Story{
		ID:           "test-1",
		DedupeKey:    "test-1",
		Title:        "Test Story",
		Summary:      "Summary",
		CanonicalURL: "https://example.com/1",
		PublishedAt:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		GUID:         "guid-1",
	}

	status, err := sqlite.Upsert(db, story)
	if err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}
	if status != domain.UpsertCreated {
		t.Errorf("Upsert status = %v, want %v", status, domain.UpsertCreated)
	}

	count, err := sqlite.Count(db)
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}

	stories, err := sqlite.List(db)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(stories) != 1 {
		t.Fatalf("expected 1 story, got %d", len(stories))
	}
	if stories[0].Title != "Test Story" {
		t.Errorf("story title = %q, want %q", stories[0].Title, "Test Story")
	}

	story.Title = "Test Story Updated"
	status2, err := sqlite.Upsert(db, story)
	if err != nil {
		t.Fatalf("second Upsert failed: %v", err)
	}
	if status2 != domain.UpsertUpdated {
		t.Errorf("second Upsert status = %v, want %v", status2, domain.UpsertUpdated)
	}

	count2, err := sqlite.Count(db)
	if err != nil {
		t.Fatalf("second Count failed: %v", err)
	}
	if count2 != 1 {
		t.Errorf("second count = %d, want 1", count2)
	}
}

func TestMigrateDB_Idempotent(t *testing.T) {
	tmpfile := t.TempDir() + "/migrate-test.db"
	db, err := sql.Open("sqlite", "file:"+tmpfile)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	versions, err := sqlite.MigrateDB(db)
	if err != nil {
		t.Fatalf("first MigrateDB failed: %v", err)
	}
	if len(versions) != 10 {
		t.Errorf("first MigrateDB returned %d migrations, want 10", len(versions))
	}

	versions2, err := sqlite.MigrateDB(db)
	if err != nil {
		t.Fatalf("second MigrateDB failed: %v", err)
	}
	if len(versions2) != 0 {
		t.Errorf("second MigrateDB returned %d migrations, want 0", len(versions2))
	}
}

func TestConfigError(t *testing.T) {
	ce := &ConfigError{
		Msg:    "test error",
		Solve:  "try again",
		Secret: false,
	}
	msg := ce.Error()
	if !strings.Contains(msg, "test error") || !strings.Contains(msg, "try again") {
		t.Errorf("ConfigError.Error() = %q", msg)
	}
}

func TestRepository_FileDB(t *testing.T) {
	tmpfile := t.TempDir() + "/repo-test.db"

	repo, err := NewRepository("file:"+tmpfile, "")
	if err != nil {
		t.Fatalf("NewRepository(file:...) failed: %v", err)
	}

	story := domain.Story{
		ID:           "test-1",
		DedupeKey:    "test-1",
		Title:        "Test Story",
		Summary:      "Summary",
		CanonicalURL: "https://example.com/1",
		PublishedAt:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		GUID:         "guid-1",
	}

	status, err := repo.Upsert(story)
	if err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}
	if status != domain.UpsertCreated {
		t.Errorf("Upsert status = %v, want %v", status, domain.UpsertCreated)
	}

	count, err := repo.Count()
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}

	stories, err := repo.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(stories) != 1 {
		t.Fatalf("expected 1 story, got %d", len(stories))
	}

	story.Title = "Updated"
	status2, err := repo.Upsert(story)
	if err != nil {
		t.Fatalf("second Upsert failed: %v", err)
	}
	if status2 != domain.UpsertUpdated {
		t.Errorf("second Upsert status = %v, want %v", status2, domain.UpsertUpdated)
	}

	count2, err := repo.Count()
	if err != nil {
		t.Fatalf("second Count failed: %v", err)
	}
	if count2 != 1 {
		t.Errorf("second count = %d, want 1", count2)
	}

	// Turso Repository has Close but domain.Repository does not
	// Test Close on concrete type
	type closer interface {
		Close() error
	}
	if c, ok := repo.(closer); ok {
		if err := c.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	}
}
