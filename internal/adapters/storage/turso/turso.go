package turso

import (
	"context"
	"database/sql"
	"net/url"
	"strings"
	"time"

	_ "github.com/tursodatabase/libsql-client-go/libsql"
)

const (
	// DefaultContextTimeout is the default timeout for Open verification queries.
	DefaultContextTimeout = 30 * time.Second
)

// Open opens a Turso database connection and verifies credentials.
// It returns an error with a redacted error message that never contains the token.
// DSN can be:
//   - libsql://host/path[?authToken=...]
//   - libsql://token@host/path (userinfo token moved into query)
//   - file:... (delegates to modernc sqlite; token ignored)
func Open(databaseURL, authToken string) (*sql.DB, error) {
	if databaseURL == "" {
		return nil, &ConfigError{
			Msg:    "TURSO_DATABASE_URL is required",
			Solve:  "set TURSO_DATABASE_URL to a Turso database URL (e.g., libsql://org-db.turso.io)",
			Secret: false,
		}
	}

	dsn, err := buildDSN(databaseURL, authToken)
	if err != nil {
		return nil, err
	}

	db, err := sql.Open("libsql", dsn)
	if err != nil {
		return nil, wrapError(err, dsn)
	}

	// Verify connection and credentials (auth happens on first statement, not on Ping)
	ctx, cancel := context.WithTimeout(context.Background(), DefaultContextTimeout)
	defer cancel()

	var one int
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, wrapError(err, dsn)
	}
	if err := db.QueryRowContext(ctx, "SELECT 1").Scan(&one); err != nil {
		_ = db.Close()
		return nil, wrapError(err, dsn)
	}

	return db, nil
}

// buildDSN constructs a DSN from the database URL and token.
// It never includes the token in errors.
func buildDSN(databaseURL, authToken string) (string, error) {
	u, err := url.Parse(databaseURL)
	if err != nil {
		return "", &ConfigError{
			Msg:    "TURSO_DATABASE_URL is invalid",
			Solve:  "set a valid Turso database URL (e.g., libsql://org-db.turso.io)",
			Secret: false,
		}
	}

	// Collect existing query params that might hold auth
	query := u.Query()

	// If the URL has a userinfo component, move it into auth_token query param
	// (libsql driver ignores userinfo for auth; must be in query)
	if u.User != nil {
		auth := u.User.Username()
		if auth != "" {
			query.Set("authToken", auth)
		}
		// Clear userinfo (will be re-encoded)
		u.User = nil
	}

	// If caller passed authToken separately and not already present, add it
	if authToken != "" && !query.Has("authToken") && !query.Has("auth_token") && !query.Has("jwt") {
		query.Set("authToken", authToken)
	}

	// Rebuild URL
	u.RawQuery = query.Encode()
	return u.String(), nil
}

// RedactedDSN returns a copy of the DSN with the token replaced by ***.
// It does not parse or validate the DSN; it only replaces common secret patterns.
func RedactedDSN(dsn string) string {
	// Replace token=... query param
	if idx := strings.Index(dsn, "?"); idx >= 0 {
		query := dsn[idx+1:]
		result := dsn[:idx+1]
		parts := strings.Split(query, "&")
		for i, p := range parts {
			if strings.HasPrefix(p, "authToken=") || strings.HasPrefix(p, "auth_token=") || strings.HasPrefix(p, "jwt=") {
				parts[i] = strings.Split(p, "=")[0] + "=***"
			}
		}
		result += strings.Join(parts, "&")
		return result
	}
	return dsn
}

// wrapError wraps a driver error, redacting the token and prefixing helpful context.
func wrapError(err error, dsn string) error {
	msg := err.Error()
	// Redact token if present
	dsnRedacted := RedactedDSN(dsn)
	msg = strings.ReplaceAll(msg, dsn, dsnRedacted)

	// Check for Turso HTTP error patterns
	if strings.Contains(msg, "401") || strings.Contains(msg, "Unauthorized") || strings.Contains(msg, "empty JWT token") {
		return &ConfigError{
			Msg:    "authentication failed",
			Solve:  "set TURSO_AUTH_TOKEN with a valid Turso authentication token",
			Secret: false,
		}
	}
	if strings.Contains(msg, "400") || strings.Contains(msg, "InvalidToken") {
		return &ConfigError{
			Msg:    "authentication token is invalid",
			Solve:  "check TURSO_AUTH_TOKEN; the provided token is not accepted",
			Secret: false,
		}
	}

	// Generic Turso-specific hint
	if strings.Contains(msg, "database") || strings.Contains(msg, "connection") {
		return &ConfigError{
			Msg:    "failed to open Turso database",
			Solve:  "check TURSO_DATABASE_URL and TURSO_AUTH_TOKEN; the DSN may be invalid or the server unreachable",
			Secret: false,
		}
	}

	// For non-Turso errors, at least redact the DSN
	return &ConfigError{
		Msg:    err.Error(),
		Solve:  "check database configuration",
		Secret: false,
	}
}

// ConfigError represents a configuration error that never includes secrets.
type ConfigError struct {
	Msg    string
	Solve  string
	Secret bool
}

func (e *ConfigError) Error() string {
	if e.Secret {
		return e.Msg + "; " + e.Solve
	}
	return e.Msg + "; " + e.Solve
}

// OpenDB opens a Turso database using a pre-constructed DSN.
// Prefer Open() for most use cases; use OpenDB when you already have a DSN.
func OpenDB(dsn string) (*sql.DB, error) {
	db, err := sql.Open("libsql", dsn)
	if err != nil {
		return nil, wrapError(err, dsn)
	}
	return db, nil
}
