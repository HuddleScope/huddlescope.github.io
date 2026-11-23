package turso

import (
	"database/sql"

	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/storage/sqlite"
	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

// NewRepository opens a Turso database and returns a domain.Repository.
// It runs schema migrations against the database.
func NewRepository(databaseURL, authToken string) (domain.Repository, error) {
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

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, wrapError(err, dsn)
	}

	// Apply migrations on the open handle
	if _, err := sqlite.MigrateDB(db); err != nil {
		_ = db.Close()
		return nil, err
	}

	return &Repository{db: db}, nil
}

// Repository implements domain.Repository using Turso via the libsql driver.
type Repository struct {
	db *sql.DB
}

// Upsert implements domain.Repository.Upsert.
func (r *Repository) Upsert(story domain.Story) (domain.UpsertStatus, error) {
	return sqlite.Upsert(r.db, story)
}

// UpsertArticle implements domain.Repository.UpsertArticle.
func (r *Repository) UpsertArticle(article domain.Article) error {
	return sqlite.UpsertArticle(r.db, article)
}

// List implements domain.Repository.List.
func (r *Repository) List() ([]domain.Story, error) {
	return sqlite.List(r.db)
}

// Count implements domain.Repository.Count.
func (r *Repository) Count() (int, error) {
	return sqlite.Count(r.db)
}

// Migrate applies schema migrations.
func (r *Repository) Migrate() error {
	_, err := sqlite.MigrateDB(r.db)
	return err
}

// Close closes the database connection.
func (r *Repository) Close() error {
	return r.db.Close()
}

// DB returns the underlying *sql.DB handle so companion repositories (e.g. the
// correction-rule repository) can share the connection.
func (r *Repository) DB() *sql.DB {
	return r.db
}
