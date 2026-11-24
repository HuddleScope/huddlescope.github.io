package e2e

import (
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

// failingRepository implements domain.Repository so that every read or write
// fails with a fixed error. It models a storage backend outage (e.g. Turso
// unreachable) at the Go level: the application cannot list stories, upsert
// new entries, or persist articles, so both the pipeline and the snapshot
// export must fail cleanly without touching the published snapshot file.
type failingRepository struct {
	err error
}

func newFailingRepository() *failingRepository {
	return &failingRepository{
		err: errors.New("turso: connection refused (simulated outage)"),
	}
}

func (r *failingRepository) Upsert(story domain.Story) (domain.UpsertStatus, error) {
	return domain.UpsertUnchanged, r.err
}

func (r *failingRepository) UpsertArticle(article domain.Article) error {
	return r.err
}

func (r *failingRepository) List() ([]domain.Story, error) {
	return nil, r.err
}

func (r *failingRepository) Count() (int, error) {
	return 0, r.err
}

// fileBytes reads a file and returns its raw content plus an error if the file
// is missing. Tests use it to assert byte-identity of the published snapshot
// before and after a failure.
func fileBytes(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read %s: %v", path, err)
	}
	return b
}

// describeErrors renders a slice of errors as a single line for readable
// failure messages.
func describeErrors(errs []error) string {
	parts := make([]string, 0, len(errs))
	for _, e := range errs {
		parts = append(parts, e.Error())
	}
	if len(parts) == 0 {
		return "(none)"
	}
	return fmt.Sprintf("%v", parts)
}
