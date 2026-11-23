package types

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/heartbeat"
	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/publish"
	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/rss"
	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/storage"
	"github.com/HuddleScope/huddlescope.github.io/internal/application"
	"github.com/HuddleScope/huddlescope.github.io/internal/config"
	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
)

// ExitError is a sentinel error type for exit code communication.
type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string {
	return e.Err.Error()
}

func (e *ExitError) Unwrap() error {
	return e.Err
}

// ExitCode returns the exit code.
func (e *ExitError) ExitCode() int {
	return e.Code
}

// NewExitError creates a new ExitError.
func NewExitError(code int, err error) *ExitError {
	return &ExitError{Code: code, Err: err}
}

// RunResult represents the result of running a command.
type RunResult struct {
	ExitCode int
	Error    error
}

// IsSuccess returns true if the command succeeded.
func (r *RunResult) IsSuccess() bool {
	return r.ExitCode == 0
}

// IsFailure returns true if the command failed.
func (r *RunResult) IsFailure() bool {
	return r.ExitCode != 0
}

// LoadConfigFunc is a function type for loading configuration.
type LoadConfigFunc func(path string) (config.Config, error)

// DefaultLoadConfig is the default configuration loader.
func DefaultLoadConfig(path string) (config.Config, error) {
	return config.Load(path)
}

// NewRepoFunc is a function type for creating a repository.
type NewRepoFunc func(cfg config.StorageConfig) (domain.Repository, error)

// DefaultNewRepository is the default repository constructor.
func DefaultNewRepository(cfg config.StorageConfig) (domain.Repository, error) {
	return storage.NewRepository(cfg)
}

// NewAppFunc is a function type for creating an application.
type NewAppFunc func(opts []application.Option) *application.Application

// DefaultNewApp is the default application constructor.
func DefaultNewApp(opts []application.Option) *application.Application {
	return application.New(opts...)
}

// NewPublisherFunc is a function type for creating a publisher.
type NewPublisherFunc func() *publish.LogPublisher

// DefaultNewPublisher is the default publisher constructor.
func DefaultNewPublisher() *publish.LogPublisher {
	return publish.NewLogPublisher()
}

// NewFetcherFunc is a function type for creating a fetcher.
type NewFetcherFunc func(source domain.FeedSource, opts ...rss.Option) domain.FeedFetcher

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

// NowFunc is a function type for getting the current time.
type NowFunc func() time.Time

// DefaultNow is the default time function.
func DefaultNow() time.Time {
	return time.Now().UTC()
}

// LoadHeartbeatFunc is a function type for loading heartbeat data.
type LoadHeartbeatFunc func(path string) (*domain.Heartbeat, bool, error)

// DefaultLoadHeartbeat is the default heartbeat loader.
func DefaultLoadHeartbeat(path string) (*domain.Heartbeat, bool, error) {
	return heartbeat.Load(path)
}

// LoadHeartbeat loads heartbeat data from a file.
func LoadHeartbeat(path string) (*domain.Heartbeat, bool, error) {
	return heartbeat.Load(path)
}

// Deps holds dependencies for command wiring.
type Deps struct {
	Output        io.Writer
	ErrorOutput   io.Writer
	LoadConfig    LoadConfigFunc
	NewRepository NewRepoFunc
	NewApp        NewAppFunc
	NewPublisher  NewPublisherFunc
	NewFetcher    NewFetcherFunc
	Now           NowFunc
	LoadHeartbeat LoadHeartbeatFunc
}

// NewDeps creates a new deps instance with options.
func NewDeps(opts ...Option) *Deps {
	d := &Deps{
		Output:        os.Stdout,
		ErrorOutput:   os.Stderr,
		LoadConfig:    DefaultLoadConfig,
		NewRepository: DefaultNewRepository,
		NewApp:        DefaultNewApp,
		NewPublisher:  DefaultNewPublisher,
		NewFetcher:    DefaultNewFetcher,
		Now:           DefaultNow,
		LoadHeartbeat: DefaultLoadHeartbeat,
	}
	for _, opt := range opts {
		opt(d)
	}
	return d
}

// DefaultDeps returns a deps instance with default values.
func DefaultDeps() *Deps {
	return NewDeps()
}

// Option is a functional option for configuring Deps.
type Option func(*Deps)

// WithOutput sets the output writer.
func WithOutput(w io.Writer) Option {
	return func(d *Deps) {
		d.Output = w
	}
}

// WithErrorOutput sets the error writer.
func WithErrorOutput(w io.Writer) Option {
	return func(d *Deps) {
		d.ErrorOutput = w
	}
}

// WithLoadConfig sets the configuration loader.
func WithLoadConfig(fn LoadConfigFunc) Option {
	return func(d *Deps) {
		d.LoadConfig = fn
	}
}

// WithNewRepository sets the repository constructor.
func WithNewRepository(fn NewRepoFunc) Option {
	return func(d *Deps) {
		d.NewRepository = fn
	}
}

// WithNewApp sets the application constructor.
func WithNewApp(fn NewAppFunc) Option {
	return func(d *Deps) {
		d.NewApp = fn
	}
}

// WithNewPublisher sets the publisher constructor.
func WithNewPublisher(fn NewPublisherFunc) Option {
	return func(d *Deps) {
		d.NewPublisher = fn
	}
}

// WithNewFetcher sets the fetcher constructor.
func WithNewFetcher(fn NewFetcherFunc) Option {
	return func(d *Deps) {
		d.NewFetcher = fn
	}
}

// WithNow sets the time function.
func WithNow(fn NowFunc) Option {
	return func(d *Deps) {
		d.Now = fn
	}
}

// WithLoadHeartbeat sets the heartbeat loader.
func WithLoadHeartbeat(fn LoadHeartbeatFunc) Option {
	return func(d *Deps) {
		d.LoadHeartbeat = fn
	}
}

// Validate checks if the deps are properly configured.
func (d *Deps) Validate() error {
	if d.LoadConfig == nil {
		return fmt.Errorf("LoadConfig is nil")
	}
	if d.NewRepository == nil {
		return fmt.Errorf("NewRepository is nil")
	}
	if d.NewApp == nil {
		return fmt.Errorf("NewApp is nil")
	}
	if d.NewPublisher == nil {
		return fmt.Errorf("NewPublisher is nil")
	}
	if d.NewFetcher == nil {
		return fmt.Errorf("NewFetcher is nil")
	}
	if d.Now == nil {
		return fmt.Errorf("now is nil")
	}
	if d.LoadHeartbeat == nil {
		return fmt.Errorf("LoadHeartbeat is nil")
	}
	return nil
}
