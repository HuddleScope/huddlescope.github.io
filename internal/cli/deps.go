package cli

import (
	"time"

	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/publish"
	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/rss"
	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/storage"
	"github.com/HuddleScope/huddlescope.github.io/internal/application"
	"github.com/HuddleScope/huddlescope.github.io/internal/config"
	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
	"github.com/HuddleScope/huddlescope.github.io/internal/types"
)

// Re-export types from internal/types for convenience
type (
	ExitError               = types.ExitError
	RunResult               = types.RunResult
	LoadConfigFunc          = types.LoadConfigFunc
	NewRepoFunc             = types.NewRepoFunc
	NewAppFunc              = types.NewAppFunc
	NewPublisherFunc        = types.NewPublisherFunc
	NewFetcherFunc          = types.NewFetcherFunc
	NowFunc                 = types.NowFunc
	LoadHeartbeatFunc       = types.LoadHeartbeatFunc
	NewRetentionServiceFunc = types.NewRetentionServiceFunc
	Deps                    = types.Deps
	Option                  = types.Option
)

// NewExitError creates a new ExitError.
func NewExitError(code int, err error) *ExitError {
	return types.NewExitError(code, err)
}

// NewDeps creates a new deps instance with options.
func NewDeps(opts ...Option) *Deps {
	return types.NewDeps(opts...)
}

// DefaultDeps returns a deps instance with default values.
func DefaultDeps() *Deps {
	return types.DefaultDeps()
}

// DefaultLoadConfig is the default configuration loader.
func DefaultLoadConfig(path string) (config.Config, error) {
	return config.Load(path)
}

// DefaultNewRepository is the default repository constructor.
func DefaultNewRepository(cfg config.StorageConfig) (domain.Repository, error) {
	return storage.NewRepository(cfg)
}

// DefaultNewApp is the default application constructor.
func DefaultNewApp(opts []application.Option) *application.Application {
	return application.New(opts...)
}

// DefaultNewPublisher is the default publisher constructor.
func DefaultNewPublisher() *publish.LogPublisher {
	return publish.NewLogPublisher()
}

// DefaultNewFetcher is the default fetcher constructor.
func DefaultNewFetcher(source domain.FeedSource, opts ...rss.Option) domain.FeedFetcher {
	rateLimiter := rss.NewHostRateLimiter(1 * time.Second)
	etagCache := rss.NewETagCache()
	return rss.NewHTTPFetcher(source,
		rss.WithUserAgent("huddlescope/0.1"),
		rss.WithDefaultTimeout(30*time.Second),
		rss.WithRateLimiter(rateLimiter),
		rss.WithETagCache(etagCache),
	)
}

// DefaultNow is the default time function.
func DefaultNow() time.Time {
	return time.Now().UTC()
}

// DefaultLoadHeartbeat is the default heartbeat loader.
func DefaultLoadHeartbeat(path string) (*domain.Heartbeat, bool, error) {
	return types.LoadHeartbeat(path)
}

// WithOutput sets the output writer.
func WithOutput(w interface{ Write([]byte) (int, error) }) Option {
	return types.WithOutput(w)
}

// WithErrorOutput sets the error writer.
func WithErrorOutput(w interface{ Write([]byte) (int, error) }) Option {
	return types.WithErrorOutput(w)
}

// WithLoadConfig sets the configuration loader.
func WithLoadConfig(fn LoadConfigFunc) Option {
	return types.WithLoadConfig(fn)
}

// WithNewRepository sets the repository constructor.
func WithNewRepository(fn NewRepoFunc) Option {
	return types.WithNewRepository(fn)
}

// WithNewApp sets the application constructor.
func WithNewApp(fn NewAppFunc) Option {
	return types.WithNewApp(fn)
}

// WithNewPublisher sets the publisher constructor.
func WithNewPublisher(fn NewPublisherFunc) Option {
	return types.WithNewPublisher(fn)
}

// WithNewFetcher sets the fetcher constructor.
func WithNewFetcher(fn NewFetcherFunc) Option {
	return types.WithNewFetcher(fn)
}

// WithNow sets the time function.
func WithNow(fn NowFunc) Option {
	return types.WithNow(fn)
}

// WithLoadHeartbeat sets the heartbeat loader.
func WithLoadHeartbeat(fn LoadHeartbeatFunc) Option {
	return types.WithLoadHeartbeat(fn)
}

// WithNewRetentionService sets the retention service constructor (issue #120).
func WithNewRetentionService(fn NewRetentionServiceFunc) Option {
	return func(d *Deps) {
		d.NewRetentionService = fn
	}
}
