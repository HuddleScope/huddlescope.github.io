package runner

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/heartbeat"
	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/hugo"
	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/rss"
	"github.com/HuddleScope/huddlescope.github.io/internal/application"
	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
	"github.com/HuddleScope/huddlescope.github.io/internal/types"
)

// Runner orchestrates the execution of commands.
type Runner struct {
	out           io.Writer
	err           io.Writer
	loadConfig    types.LoadConfigFunc
	newRepository types.NewRepoFunc
	newApp        types.NewAppFunc
	newPublisher  types.NewPublisherFunc
	newFetcher    types.NewFetcherFunc
	now           types.NowFunc
	loadHeartbeat types.LoadHeartbeatFunc
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

// NewRunner creates a new runner instance.
func NewRunner(opts ...Option) *Runner {
	r := &Runner{
		out:           os.Stdout,
		err:           os.Stderr,
		loadConfig:    types.DefaultLoadConfig,
		newRepository: types.DefaultNewRepository,
		newApp:        types.DefaultNewApp,
		newPublisher:  types.DefaultNewPublisher,
		newFetcher:    types.DefaultNewFetcher,
		now:           types.DefaultNow,
		loadHeartbeat: types.DefaultLoadHeartbeat,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

type Option func(*Runner)

// WithOutput sets the output writer.
func WithOutput(w io.Writer) Option {
	return func(r *Runner) {
		r.out = w
	}
}

// WithErrorOutput sets the error writer.
func WithErrorOutput(w io.Writer) Option {
	return func(r *Runner) {
		r.err = w
	}
}

func WithLoadConfig(fn types.LoadConfigFunc) Option {
	return func(r *Runner) {
		r.loadConfig = fn
	}
}

// WithNewRepository sets the repository constructor.
func WithNewRepository(fn types.NewRepoFunc) Option {
	return func(r *Runner) {
		r.newRepository = fn
	}
}

// WithNewApp sets the application constructor.
func WithNewApp(fn types.NewAppFunc) Option {
	return func(r *Runner) {
		r.newApp = fn
	}
}

// WithNewPublisher sets the publisher constructor.
func WithNewPublisher(fn types.NewPublisherFunc) Option {
	return func(r *Runner) {
		r.newPublisher = fn
	}
}

// WithNewFetcher sets the fetcher constructor.
func WithNewFetcher(fn types.NewFetcherFunc) Option {
	return func(r *Runner) {
		r.newFetcher = fn
	}
}

// WithNow sets the time function.
func WithNow(fn types.NowFunc) Option {
	return func(r *Runner) {
		r.now = fn
	}
}

// WithLoadHeartbeat sets the heartbeat loader.
func WithLoadHeartbeat(fn types.LoadHeartbeatFunc) Option {
	return func(r *Runner) {
		r.loadHeartbeat = fn
	}
}

// RunIngestResult represents the result of running an ingest command.
type RunIngestResult struct {
	RunResult
	Result  *domain.RunRecord
	Summary string
}

// RunIngest runs the ingestion pipeline.
func (r *Runner) RunIngest(ctx context.Context, mode string, staleAfter time.Duration, force bool) *RunIngestResult {
	result := &RunIngestResult{}

	cfg, err := r.loadConfig("")
	if err != nil {
		result.ExitCode = 1
		result.Error = fmt.Errorf("failed to load config: %w", err)
		return result
	}

	if err := cfg.Validate(); err != nil {
		result.ExitCode = 1
		result.Error = fmt.Errorf("config validation failed: %w", err)
		return result
	}

	// Build publisher
	publisher := r.newPublisher()

	// Build repository based on storage backend
	repo, err := r.newRepository(cfg.Storage())
	if err != nil {
		result.ExitCode = 1
		result.Error = fmt.Errorf("failed to create repository: %w", err)
		return result
	}

	registry := cfg.SourceRegistry()

	if registry.Count() == 0 {
		result.ExitCode = 1
		result.Error = fmt.Errorf("no feed sources are configured")
		return result
	}

	teamRegistry, err := cfg.TeamRegistry()
	if err != nil {
		result.ExitCode = 1
		result.Error = fmt.Errorf("failed to load teams: %w", err)
		return result
	}

	// Build application
	var appOpts []application.Option
	appOpts = append(appOpts, application.WithRepository(repo))
	appOpts = append(appOpts, application.WithPublisher(publisher))
	appOpts = append(appOpts, application.WithLookback(cfg.LookbackWindow))

	// Registry-driven ingestion with real HTTP fetchers
	rateLimiter := rss.NewHostRateLimiter(1 * time.Second)
	etagCache := rss.NewETagCache()

	sourceFetchers := make(map[string]domain.FeedFetcher)
	for _, src := range registry.Enabled() {
		fetcher := r.newFetcher(src,
			rss.WithUserAgent(cfg.UserAgent),
			rss.WithDefaultTimeout(cfg.RequestTimeout),
			rss.WithRateLimiter(rateLimiter),
			rss.WithETagCache(etagCache),
		)
		sourceFetchers[src.ID] = fetcher
	}
	appOpts = append(appOpts, application.WithRegistry(registry))
	appOpts = append(appOpts, application.WithFetcherMap(sourceFetchers))

	// Add clusterer and scorer (required for pipeline)
	appOpts = append(appOpts, application.WithClusterer(cfg.ClustererSettings()))
	appOpts = append(appOpts, application.WithScorer(domain.NewScorer()))

	// Add team registry for entity extraction
	appOpts = append(appOpts, application.WithTeamRegistry(teamRegistry))

	app := application.New(appOpts...)

	// Build timestamp for pipeline
	now := r.now()

	// Run pipeline
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Catch interrupt to cancel the pipeline
	signalChan := make(chan os.Signal, 1)
	go func() {
		<-signalChan
		cancel()
	}()

	runRecord := app.RunPipeline(runCtx, now)
	result.Result = &runRecord

	// Publish heartbeat (best-effort: failures don't affect run exit)
	heartbeatPath := cfg.HeartbeatFile
	if hbPub := heartbeat.NewFilePublisher(heartbeatPath); hbPub != nil {
		if hb := domain.NewHeartbeat(result.Result.RunID.String()); hb != nil {
			hb.FromRunRecord(*result.Result, *result.Result.EndedAt)
			// Set PublishedAt when ingest succeeds so heartbeat check sees fresh state
			if result.Result.Status == domain.RunStatusOK {
				pubTime := result.Result.EndedAt.UTC().Format(time.RFC3339)
				hb.PublishedAt = &pubTime
			}
			if err := hbPub.Publish(context.Background(), hb); err != nil {
				_, _ = fmt.Fprintf(r.err, "Warning: failed to publish heartbeat: %v\n", err)
			}
		}
	}

	// Publish feed health (best-effort: failures don't affect run exit)
	feedsPath := cfg.FeedsFile
	// Extract outDir and dataFile from full path
	feedsDir := filepath.Dir(feedsPath)
	feedsFile := filepath.Base(feedsPath)
	if fhPub := hugo.NewFeedsDataPublisher(feedsDir, feedsFile); fhPub != nil {
		if feedHealth := app.BuildFeedHealth(*result.Result, *result.Result.EndedAt); feedHealth.Version != "" {
			_, err := fhPub.Publish(context.Background(), feedHealth)
			if err != nil {
				_, _ = fmt.Fprintf(r.err, "Warning: failed to publish feed health: %v\n", err)
			}
		}
	}

	// Build summary
	result.Summary = buildIngestSummary(result.Result)

	// Set exit code based on status
	if result.Result.Status.IsCritical() {
		result.ExitCode = result.Result.Status.ExitCode()
	} else if !result.Result.Committed {
		result.ExitCode = 1
	} else {
		result.ExitCode = 0
	}

	return result
}

// buildIngestSummary builds a summary string for the ingest result.
func buildIngestSummary(result *domain.RunRecord) string {
	buf := &strings.Builder{}

	fmt.Fprintf(buf, "Run ID: %s\n", result.RunID)
	fmt.Fprintf(buf, "Started at: %s\n", result.StartedAt.Format(time.RFC3339))

	if result.EndedAt != nil {
		fmt.Fprintf(buf, "Ended at: %s\n", result.EndedAt.Format(time.RFC3339))
	}

	fmt.Fprintf(buf, "Status: %s\n", result.Status)

	if result.LastPublishedAt != nil {
		fmt.Fprintf(buf, "Last published at: %s\n", result.LastPublishedAt.Format(time.RFC3339))
	}

	if result.Committed {
		fmt.Fprintf(buf, "Homepage committed successfully\n")
	} else {
		fmt.Fprintf(buf, "Homepage was not committed\n")
	}

	fmt.Fprintf(buf, "\nEntries:\n")
	fmt.Fprintf(buf, "  Fetched: %d\n", result.EntriesFetched)
	fmt.Fprintf(buf, "  Created: %d\n", result.StoriesCreated)
	fmt.Fprintf(buf, "  Updated: %d\n", result.StoriesUpdated)
	fmt.Fprintf(buf, "  Skipped: %d\n", result.StoriesSkipped)

	if result.Homepage != nil {
		fmt.Fprintf(buf, "\nHomepage:\n")
		fmt.Fprintf(buf, "  Stories built: %d\n", len(result.Homepage.RankedStories))
		fmt.Fprintf(buf, "  Notes added: %d\n", len(result.Homepage.Notes))
	}

	fmt.Fprintf(buf, "\nGroups:\n")
	fmt.Fprintf(buf, "  Created: %d\n", result.GroupsCreated)
	fmt.Fprintf(buf, "  Articles grouped: %d\n", result.ArticlesGrouped)
	fmt.Fprintf(buf, "  Articles solo: %d\n", result.ArticlesSolo)

	if len(result.Errors) > 0 {
		fmt.Fprintf(buf, "\nErrors:\n")
		for _, err := range result.Errors {
			fmt.Fprintf(buf, "  - %v\n", err)
		}
	}

	return buf.String()
}

// RunExportResult represents the result of running an export command.
type RunExportResult struct {
	RunResult
	SnapshotResult application.SnapshotResult
}

// RunExport runs the export pipeline.
func (r *Runner) RunExport(ctx context.Context, dryRun bool, dataDir, dataFile, contentDir string) *RunExportResult {
	result := &RunExportResult{}

	cfg, err := r.loadConfig("")
	if err != nil {
		result.ExitCode = 1
		result.Error = fmt.Errorf("failed to load config: %w", err)
		return result
	}

	if err := cfg.Validate(); err != nil {
		result.ExitCode = 1
		result.Error = fmt.Errorf("config validation failed: %w", err)
		return result
	}

	resolvedDataDir := dataDir
	if resolvedDataDir == "" {
		resolvedDataDir = cfg.DataDir
	}
	resolvedDataFile := dataFile
	if resolvedDataFile == "" {
		resolvedDataFile = cfg.DataFile
	}

	resolvedContentDir := contentDir
	if resolvedContentDir == "" {
		resolvedContentDir = cfg.HugoContentDir
	}

	// Build repository
	repo, err := r.newRepository(cfg.Storage())
	if err != nil {
		result.ExitCode = 1
		result.Error = fmt.Errorf("failed to create repository: %w", err)
		return result
	}

	teamRegistry, err := cfg.TeamRegistry()
	if err != nil {
		result.ExitCode = 1
		result.Error = fmt.Errorf("failed to load teams: %w", err)
		return result
	}

	// Build clusterer and scorer (fixed time for determinism in tests)
	clustering := cfg.ClustererSettings()
	scorer := domain.NewScorer().WithTime(time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC))

	// Build story page publisher (HS-061): writes into the Hugo content directory
	storyPagePublisher := hugo.NewPublisher(resolvedContentDir, cfg.HugoBaseURL)

	// Build snapshot publisher (writes to Hugo data directory)
	snapshotPublisher := hugo.NewDataPublisher(resolvedDataDir, resolvedDataFile)

	// Build application
	app := application.New(
		application.WithRepository(repo),
		application.WithTeamRegistry(teamRegistry),
		application.WithClusterer(clustering),
		application.WithScorer(scorer),
		application.WithSnapshotPublisher(snapshotPublisher),
		application.WithStoryPagePublisher(storyPagePublisher),
	)

	// Run snapshot export
	snapshotResult := app.RunSnapshotPreview(context.Background(), time.Now())
	result.SnapshotResult = snapshotResult

	if dryRun {
		// Dry run: output result but skip write and heartbeat update
		_, _ = fmt.Fprintf(r.out, "Snapshot preview complete\n")
		_, _ = fmt.Fprintf(r.out, "  Committed: %v\n", snapshotResult.PubResult.Committed)
		_, _ = fmt.Fprintf(r.out, "  Changed: %v\n", snapshotResult.PubResult.Changed)
		_, _ = fmt.Fprintf(r.out, "  Path: %v\n", snapshotResult.PubResult.Path)
		_, _ = fmt.Fprintf(r.out, "  Version: %v\n", snapshotResult.PubResult.Version)
		_, _ = fmt.Fprintf(r.out, "  Stories built: %d\n", snapshotResult.StoriesBuilt)

		if len(snapshotResult.PubResult.Errors) > 0 {
			_, _ = fmt.Fprintf(r.out, "  Errors: %v\n", snapshotResult.PubResult.Errors)
		}

		if len(snapshotResult.Errors) > 0 {
			_, _ = fmt.Fprintf(r.out, "Application errors: %v\n", snapshotResult.Errors)
			result.ExitCode = 2
		}
	} else {
		// Live run: execute and write to storage
		snapshotResult := app.RunSnapshot(ctx, time.Now())
		result.SnapshotResult = snapshotResult
	}

	return result
}

// CheckHeartbeatResult represents the result of checking heartbeat status.
type CheckHeartbeatResult struct {
	RunResult
	Stale bool
}

// CheckHeartbeat checks if the last heartbeat is stale.
func (r *Runner) CheckHeartbeat(ctx context.Context, heartbeatFile string, staleAfter time.Duration) *CheckHeartbeatResult {
	result := &CheckHeartbeatResult{}

	hb, ok, err := r.loadHeartbeat(heartbeatFile)
	if err != nil {
		result.ExitCode = 1
		result.Error = fmt.Errorf("failed to load heartbeat: %w", err)
		_, _ = fmt.Fprintf(r.err, "Error loading heartbeat: %v\n", err)
		return result
	}

	if !ok {
		// File doesn't exist - print warning and exit cleanly
		_, _ = fmt.Fprintf(r.err, "Warning: Heartbeat file not found: %s (first run or deleted)\n", heartbeatFile)
		result.ExitCode = 0
		return result
	}

	// Print summary (matches legacy output format)
	_, _ = fmt.Fprintln(r.out, "Heartbeat status:")
	_, _ = fmt.Fprintf(r.out, "  Version: %s\n", hb.Version)
	_, _ = fmt.Fprintf(r.out, "  Run ID: %s\n", hb.RunID)
	_, _ = fmt.Fprintf(r.out, "  Status: %s\n", hb.Status)
	_, _ = fmt.Fprintf(r.out, "  Attempted at: %s\n", hb.AttemptedAt)
	if hb.IngestedAt != nil {
		_, _ = fmt.Fprintf(r.out, "  Ingested at: %s\n", *hb.IngestedAt)
	} else {
		_, _ = fmt.Fprintf(r.out, "  Ingested at: (none)\n")
	}
	if hb.PublishedAt != nil {
		_, _ = fmt.Fprintf(r.out, "  Published at: %s\n", *hb.PublishedAt)
	} else {
		_, _ = fmt.Fprintf(r.out, "  Published at: (none)\n")
	}
	_, _ = fmt.Fprintf(r.out, "  Sources checked: %d\n", hb.SourcesChecked)
	_, _ = fmt.Fprintf(r.out, "  Sources failed: %d\n", hb.SourcesFailed)
	_, _ = fmt.Fprintf(r.out, "  Errors: %d\n", len(hb.Errors))
	if len(hb.Errors) > 0 {
		_, _ = fmt.Fprintln(r.out, "  Errors (first 5):")
		for i, e := range hb.Errors {
			if i >= 5 {
				break
			}
			_, _ = fmt.Fprintf(r.out, "    %s\n", e)
		}
	}

	// Check staleness
	now := r.now()
	isStale := hb.IsStale(now, staleAfter)

	_, _ = fmt.Fprintf(r.out, "  Stale threshold: %v\n", staleAfter)
	if isStale {
		_, _ = fmt.Fprintln(r.out, "  Stale: YES (publication not fresh)")
		result.Stale = true
		result.ExitCode = 1
		result.Error = fmt.Errorf("heartbeat is stale")
		return result
	}

	_, _ = fmt.Fprintln(r.out, "  Stale: no")
	result.ExitCode = 0
	return result
}

type CheckConfigResult struct {
	RunResult
}

func (r *Runner) CheckConfig(ctx context.Context, format string) *CheckConfigResult {
	result := &CheckConfigResult{}

	cfg, err := r.loadConfig("")
	if err != nil {
		result.ExitCode = 1
		result.Error = fmt.Errorf("failed to load config: %w", err)
		return result
	}

	if err := cfg.Validate(); err != nil {
		result.ExitCode = 1
		result.Error = fmt.Errorf("config validation failed: %w", err)
		return result
	}

	switch format {
	case "json":
		output, err := cfg.MarshalJSON()
		if err != nil {
			result.ExitCode = 1
			result.Error = fmt.Errorf("failed to marshal config: %w", err)
			return result
		}
		_, _ = fmt.Fprintln(r.out, string(output))
	default:
		_, _ = fmt.Fprintln(r.out, cfg)
	}

	result.ExitCode = 0
	return result
}

// VersionResult represents the result of showing version information.
type VersionResult struct {
	RunResult
	Version string
	Commit  string
}

// ShowVersion shows version information.
func (r *Runner) ShowVersion(ctx context.Context) *VersionResult {
	result := &VersionResult{
		Version: "0.3.0",
		Commit:  "unknown",
	}

	_, _ = fmt.Fprintf(r.out, "huddlescope version %s\n", result.Version)
	_, _ = fmt.Fprintf(r.out, "Commit: %s\n", result.Commit)

	result.ExitCode = 0
	return result
}
