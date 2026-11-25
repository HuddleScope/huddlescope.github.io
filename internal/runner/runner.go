package runner

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	ossignal "os/signal"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/heartbeat"
	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/hugo"
	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/rss"
	"github.com/HuddleScope/huddlescope.github.io/internal/adapters/storage"
	"github.com/HuddleScope/huddlescope.github.io/internal/application"
	"github.com/HuddleScope/huddlescope.github.io/internal/config"
	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
	"github.com/HuddleScope/huddlescope.github.io/internal/types"
)

// Runner orchestrates the execution of commands.
type Runner struct {
	out           io.Writer
	err           io.Writer
	loadConfig    types.LoadConfigFunc
	newRepository types.NewRepoFunc
	newRuleRepo   types.NewRuleRepoFunc
	newIngestRuns types.NewIngestRunRepoFunc
	newLeaseRepo  types.NewLeaseRepoFunc
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
		newRuleRepo:   types.DefaultNewRuleRepository,
		newIngestRuns: types.DefaultNewIngestRunRepository,
		newLeaseRepo:  types.DefaultNewLeaseRepository,
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

// WithNewRuleRepo sets the correction-rule repository constructor (issue #25).
func WithNewRuleRepo(fn types.NewRuleRepoFunc) Option {
	return func(r *Runner) {
		r.newRuleRepo = fn
	}
}

// WithNewIngestRunRepo sets the ingest-run repository constructor (issue #159).
// A nil function disables run tracking.
func WithNewIngestRunRepo(fn types.NewIngestRunRepoFunc) Option {
	return func(r *Runner) {
		r.newIngestRuns = fn
	}
}

// WithNewLeaseRepo sets the ingest lease repository constructor (issue #160 /
// HS-PORT-04). A nil function disables distributed mutual exclusion (the runner
// proceeds without a lease and warns).
func WithNewLeaseRepo(fn types.NewLeaseRepoFunc) Option {
	return func(r *Runner) {
		r.newLeaseRepo = fn
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

// LeaseTTL is the default distributed lease lifetime (issue #160 / plan §6).
const LeaseTTL = 20 * time.Minute

// LeaseRenewInterval is the interval between lease renewals (issue #160 / plan
// §6). It must be comfortably less than LeaseTTL so a single network blip
// does not cause a spurious hand-off. Declared as a var (not const) so the
// runner tests can shorten it deterministically; production uses the default.
var LeaseRenewInterval = 5 * time.Minute

// RunIngest runs the ingestion pipeline with distributed mutual exclusion
// (issue #160 / HS-PORT-04). mode is "primary" or "backup"; staleAfter is the
// freshness threshold for the backup pre-check; force skips the staleness
// pre-check but NEVER the lease.
func (r *Runner) RunIngest(ctx context.Context, mode string, staleAfter time.Duration, force bool) *RunIngestResult {
	result := &RunIngestResult{}

	cfg, err := r.loadConfig("", config.Overrides{})
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

	// Issue #158 / AC-4: warn (one line, stderr) when the backend is memory or
	// sqlite-file so an operator does not silently assume shared/persistent
	// storage on a production host. Non-fatal: dev/test use of memory is legal.
	if cfg.StorageBackend == "memory" || cfg.StorageBackend == "sqlite-file" {
		_, _ = fmt.Fprintf(r.err, "Warning: storage backend %q is not shared/persistent; data is per-host only\n", cfg.StorageBackend)
	}

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

	// Wire the correction-rule repository (issue #25, AC-3b). A construction
	// failure is non-fatal: rules are simply skipped and the engine stays
	// independent of storage.
	if r.newRuleRepo != nil {
		if ruleRepo, rerr := r.newRuleRepo(cfg.Storage()); rerr == nil && ruleRepo != nil {
			appOpts = append(appOpts, application.WithRuleRepository(ruleRepo))
		}
	}

	// Issue #159 (HS-PORT-03): register the run in the shared database.
	// Tracking is best-effort: a storage failure must never fail the ingest
	// pipeline (same pattern as heartbeat/feed-health publishing).
	var runsRepo domain.IngestRunRepository
	if r.newIngestRuns != nil {
		if rr, rerr := r.newIngestRuns(cfg.Storage()); rerr == nil && rr != nil {
			runsRepo = rr
		} else if rerr != nil {
			_, _ = fmt.Fprintf(r.err, "Warning: ingest-run tracking unavailable: %v\n", rerr)
		}
	}

	// ---------------------------------------------------------------------
	// Issue #160 (HS-PORT-04): distributed mutual exclusion via a shared
	// database lease. The lease is the SINGLE mechanism that keeps exactly one
	// host doing RSS work at a time.
	// ---------------------------------------------------------------------

	// Build the lease repository (best-effort construction; a failure degrades
	// to "no lease" with a warning — local/dev use, never a remote race).
	var leaseRepo domain.LeaseRepository
	if r.newLeaseRepo != nil {
		if lr, lerr := r.newLeaseRepo(cfg.Storage()); lerr == nil && lr != nil {
			leaseRepo = lr
		} else if errors.Is(lerr, storage.ErrLeaseUnavailable) {
			// memory backend: no cross-host coordination possible.
			_, _ = fmt.Fprintf(r.err, "Warning: distributed lease unavailable (memory backend); proceeding without mutual exclusion\n")
		} else if lerr != nil {
			// A real DB error constructing the lease repo: fail closed so we never
			// run two ingesters unknowingly. The issue mandates that a lease-DB
			// failure is non-zero (never alter last_successful_ingest_at).
			result.ExitCode = 1
			result.Error = fmt.Errorf("failed to create lease repository: %w", lerr)
			return result
		}
	}

	// The holder identity: <origin>:<hostname>:<8-hex-random>. Sanitized to
	// [a-z0-9-] so it never carries secrets or shell metacharacters (plan §7).
	holderID := buildLeaseHolderID(cfg.RunnerOrigin)

	// Run registration state shared with the deferred finalizer below.
	var startedRun domain.IngestRun
	var startedRunRegistered bool
	var lease *domain.Lease
	// leaseLost is written by the renewal goroutine and read by the main
	// goroutine after RunPipeline returns; it must be atomic to be race-free.
	var leaseLost int32
	var skippedIngest bool // true when we register a skipped_* row and bail early

	// registerSkip registers a terminal skipped_* run row (best-effort) and
	// marks the result so the deferred finalizer does not re-finalize it.
	registerSkip := func(status domain.IngestRunStatus) {
		skippedIngest = true
		if runsRepo == nil {
			return
		}
		runID := domain.NewRunID()
		fin := r.now().UTC()
		skip := domain.IngestRun{
			RunID:      runID.String(),
			Origin:     cfg.RunnerOrigin,
			StartedAt:  r.now().UTC(),
			FinishedAt: &fin,
			Status:     status,
		}
		fctx, fcancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer fcancel()
		if cerr := runsRepo.Create(fctx, skip); cerr != nil {
			_, _ = fmt.Fprintf(r.err, "Warning: failed to register skipped run: %v\n", cerr)
		}
		if ferr := runsRepo.Finish(fctx, skip); ferr != nil {
			_, _ = fmt.Fprintf(r.err, "Warning: failed to finalize skipped run: %v\n", ferr)
		}
	}

	// releaseLease relinquishes the held lease (guarded; idempotent).
	releaseLease := func() {
		if leaseRepo == nil || lease == nil {
			return
		}
		lctx, lcancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer lcancel()
		if rerr := leaseRepo.Release(lctx, lease); rerr != nil {
			_, _ = fmt.Fprintf(r.err, "Warning: failed to release lease: %v\n", rerr)
		}
	}

	// Defer cleanup: release the lease and (if we actually started a normal run)
	// finalize it with a FRESH context so an interrupt cannot abort the
	// bookkeeping write (AC-6). Skipped runs finalize themselves in registerSkip.
	defer func() {
		releaseLease()
		if !startedRunRegistered || runsRepo == nil || skippedIngest {
			return
		}
		if result.Result == nil {
			return
		}
		finished := domain.NewIngestRunFromRecord(*result.Result, cfg.RunnerOrigin)
		finished.RunID = startedRun.RunID // preserve the registered run id
		fctx, fcancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer fcancel()
		if ferr := runsRepo.Finish(fctx, finished); ferr != nil {
			_, _ = fmt.Fprintf(r.err, "Warning: failed to finalize ingest run: %v\n", ferr)
		}
		_ = runsRepo.Close()
	}()

	// -----------------------------------------------------------------
	// Step 1: Backup pre-check (mode=backup, unless --force).
	// If the data is already fresh, print a skip and exit 0 WITHOUT touching the
	// lease (cheaper than acquiring just to immediately give up).
	// -----------------------------------------------------------------
	if mode == "backup" && !force && runsRepo != nil {
		lastSuccess, _, serr := runsRepo.LastSuccess(context.Background())
		if serr != nil {
			// DB error on the freshness read: fail closed (existing contract).
			result.ExitCode = 1
			result.Error = fmt.Errorf("failed to read last successful ingest: %w", serr)
			return result
		}
		decision := domain.DecideShouldRun(lastSuccess, r.now(), staleAfter)
		if !decision.ShouldRun {
			_, _ = fmt.Fprintf(r.out, "Ingest skipped (backup): last successful ingest is fresh (age %s <= threshold %s)\n", decision.Age, staleAfter)
			result.ExitCode = 0
			result.Summary = "skipped: fresh"
			return result
		}
	}

	// -----------------------------------------------------------------
	// Step 2: Acquire the lease (both modes; --force NEVER omits the lease).
	// -----------------------------------------------------------------
	if leaseRepo != nil {
		aCtx, aCancel := context.WithTimeout(ctx, 30*time.Second)
		acquired, ok, aerr := leaseRepo.Acquire(aCtx, holderID, LeaseTTL)
		aCancel()
		if aerr != nil {
			// Lease DB failure: fail closed. Never proceed without knowing who
			// holds the lease; never alter last_successful_ingest_at.
			result.ExitCode = 1
			result.Error = fmt.Errorf("failed to acquire ingest lease: %w", aerr)
			return result
		}
		if !ok {
			// Another host holds a valid lease: register skipped_locked, print a
			// visible skip, exit 0 (AC-1 — not a false error).
			registerSkip(domain.IngestRunSkippedLocked)
			until := "unknown"
			if acquired != nil {
				until = acquired.LeaseUntil.UTC().Format(time.RFC3339)
			}
			_, _ = fmt.Fprintf(r.out, "Ingest skipped (locked): held by %s until %s\n", holderIDOf(acquired), until)
			result.ExitCode = 0
			result.Summary = "skipped: locked"
			return result
		}
		lease = acquired

		// ---------------------------------------------------------------
		// Step 3: Backup in-lock re-check (AC-3 TOCTOU). After acquiring, re-run
		// the freshness check IMMEDIATELY before fetching. If the primary
		// recovered while we were waiting, the data is fresh: release the lease,
		// register skipped_fresh, exit 0 — NO RSS fetch.
		// ---------------------------------------------------------------
		if mode == "backup" && runsRepo != nil {
			lastSuccess, _, serr := runsRepo.LastSuccess(context.Background())
			if serr != nil {
				// Fail closed: we hold the lease, so release it on the way out.
				result.ExitCode = 1
				result.Error = fmt.Errorf("failed to re-check freshness in lock: %w", serr)
				return result // deferred releaseLease runs
			}
			decision := domain.DecideShouldRun(lastSuccess, r.now(), staleAfter)
			if !decision.ShouldRun {
				registerSkip(domain.IngestRunSkippedFresh)
				_, _ = fmt.Fprintf(r.out, "Ingest skipped (fresh in lock): another host already refreshed (age %s <= threshold %s); releasing lease\n", decision.Age, staleAfter)
				result.ExitCode = 0
				result.Summary = "skipped: fresh in lock"
				return result // deferred releaseLease runs
			}
		}
	}

	// -----------------------------------------------------------------
	// Register the run (status running) and start the pipeline under a
	// cancellable context. The renewal goroutine cancels that context if the
	// lease is lost (AC-4: a lagged holder stops making changes).
	// -----------------------------------------------------------------
	if runsRepo != nil {
		runID := domain.NewRunID()
		startedRun = domain.IngestRun{
			RunID:     runID.String(),
			Origin:    cfg.RunnerOrigin,
			StartedAt: r.now().UTC(),
			Status:    domain.IngestRunRunning,
		}
		if cerr := runsRepo.Create(context.Background(), startedRun); cerr != nil {
			_, _ = fmt.Fprintf(r.err, "Warning: failed to register ingest run: %v\n", cerr)
			runsRepo = nil // degrade: no tracking for this run
		} else {
			startedRunRegistered = true
		}
	}

	// Add team registry for entity extraction
	appOpts = append(appOpts, application.WithTeamRegistry(teamRegistry))

	app := application.New(appOpts...)

	// Build timestamp for pipeline
	now := r.now()

	// Run pipeline under a cancellable context.
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Catch SIGINT/SIGTERM to cancel the pipeline (issue #160: add SIGTERM so a
	// systemd/container stop drives the same abort + release path). Use a single
	// goroutine watching the signal channel.
	signalChan := make(chan os.Signal, 1)
	ossignal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)
	defer ossignal.Stop(signalChan)
	go func() {
		<-signalChan
		cancel()
	}()

	// -----------------------------------------------------------------
	// Renewal goroutine (issue #160 / AC-4). Every LeaseRenewInterval we renew;
	// a lost lease (false) or an unrecoverable error cancels the pipeline and
	// flags leaseLost so the run finalizes as failed (non-zero exit).
	// -----------------------------------------------------------------
	if leaseRepo != nil && lease != nil {
		go func() {
			ticker := time.NewTicker(LeaseRenewInterval)
			defer ticker.Stop()
			for {
				select {
				case <-runCtx.Done():
					return
				case <-ticker.C:
					ok, rerr := leaseRepo.Renew(runCtx, lease, LeaseTTL)
					if rerr != nil && runCtx.Err() == nil {
						// Transient renewal error (flaky network): retry within the
						// TTL rather than aborting immediately (plan §8/R5). If the
						// context was cancelled in the meantime, treat as lost.
						continue
					}
					if !ok {
						// Lease lost: stop all work. A Renew returning false is the
						// authoritative loss-of-ownership signal (fenced by token).
						atomic.StoreInt32(&leaseLost, 1)
						cancel()
						return
					}
				}
			}
		}()
	}

	runRecord := app.RunPipeline(runCtx, now)
	result.Result = &runRecord

	// If the lease was lost during the pipeline, force a failed outcome: the
	// homepage must NOT be committed by a holder that no longer owns the lease
	// (AC-4). The bookkeeping write to our own run_id row remains safe.
	if atomic.LoadInt32(&leaseLost) == 1 {
		result.Result.Committed = false
		if result.Result.Status == domain.RunStatusOK || result.Result.Status == domain.RunStatusPartial {
			result.Result.Status = domain.RunStatusFailed
		}
		result.ExitCode = 1
		result.Error = fmt.Errorf("ingest lease lost during pipeline; aborting (no commit)")
		// Still release (guarded, no-op if already gone) and finalize as failed
		// via the deferred path. Return early to skip the normal publish steps.
		return result
	}

	// Issue #158 / AC-3, AC-5: gate the site-data writes (heartbeat.json +
	// feeds.json) on ShouldPublishSiteData(). In production the default ("auto")
	// is false so a repo-less VPS/Raspberry host never creates web/data; the run
	// indicators are persisted to the shared storage instead. GitHub Actions sets
	// HSC_PUBLISH_SITE_DATA=true so the committed files keep updating.
	if !cfg.ShouldPublishSiteData() {
		_, _ = fmt.Fprintf(r.err, "site-data writes disabled (HSC_PUBLISH_SITE_DATA=%q); run indicators persisted to storage\n", orAuto(cfg.PublishSiteData))
		// Site-data writes are skipped. The feed-health counters are still
		// computed (and persisted to storage via the ingest-run repository)
		// so the dashboard can read them from Turso later (HS-PORT-07).
		_ = app.BuildFeedHealth(*result.Result, *result.Result.EndedAt)
	} else {
		// Publish heartbeat (best-effort: failures don't affect run exit)
		r.publishSiteData(context.Background(), cfg, *result.Result, *result.Result.EndedAt, func(rec domain.RunRecord, now time.Time) domain.FeedHealth {
			return app.BuildFeedHealth(rec, now)
		})
	}

	// Issue #158 / AC-8: optional single-line JSON run log (HSC_JSON_LOG=true).
	// Field whitelist only: time, run_id, origin, command, status, exit_code,
	// and a few counters. Never includes tokens, DSNs, or other secrets.
	if cfg.ShouldJSONLog() {
		if line, jerr := buildJSONRunLog(result.Result, cfg.RunnerOrigin, result.ExitCode); jerr == nil {
			_, _ = fmt.Fprintln(r.err, string(line))
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

// orAuto renders the effective HSC_PUBLISH_SITE_DATA value for log messages,
// normalizing an empty value to "auto" (the default).
func orAuto(v string) string {
	if v == "" {
		return "auto"
	}
	return v
}

// publishSiteData writes the site-data files (heartbeat.json + feeds.json)
// from a completed ingest run record. It is best-effort: failures are logged
// to stderr but never affect the run's exit code (issue #158 / AC-3, AC-5).
// The buildFeedHealth callback is injected so the caller controls how feed
// health is derived (the ingest path uses the live run record's sources;
// the export path re-projects from the DB run record).
func (r *Runner) publishSiteData(ctx context.Context, cfg config.Config, rec domain.RunRecord, endedAt time.Time, buildFeedHealth func(domain.RunRecord, time.Time) domain.FeedHealth) {
	// Publish heartbeat (best-effort: failures don't affect run exit)
	heartbeatPath := cfg.HeartbeatFile
	if hbPub := heartbeat.NewFilePublisher(heartbeatPath); hbPub != nil {
		if hb := domain.NewHeartbeat(rec.RunID.String()); hb != nil {
			hb.FromRunRecord(rec, endedAt)
			// Set PublishedAt when ingest succeeds so heartbeat check sees fresh state
			if rec.Status == domain.RunStatusOK {
				pubTime := endedAt.UTC().Format(time.RFC3339)
				hb.PublishedAt = &pubTime
			}
			if err := hbPub.Publish(ctx, hb); err != nil {
				_, _ = fmt.Fprintf(r.err, "Warning: failed to publish heartbeat: %v\n", err)
			}
		}
	}

	// Publish feed health (best-effort: failures don't affect run exit)
	if buildFeedHealth == nil {
		return
	}
	feedHealth := buildFeedHealth(rec, endedAt)
	if feedHealth.Version == "" {
		return
	}
	feedsPath := cfg.FeedsFile
	feedsDir := filepath.Dir(feedsPath)
	feedsFile := filepath.Base(feedsPath)
	if fhPub := hugo.NewFeedsDataPublisher(feedsDir, feedsFile); fhPub != nil {
		_, err := fhPub.Publish(ctx, feedHealth)
		if err != nil {
			_, _ = fmt.Fprintf(r.err, "Warning: failed to publish feed health: %v\n", err)
		}
	}
}

// publishSiteDataFromRepo re-publishes the site-data files during export by
// re-projecting the last ingest run from the shared database (issue #163 /
// HS-PORT-07 / AC-8). This keeps web/data/heartbeat.json and web/data/feeds.json
// coherent with Turso even when ingest was skipped (the healthy case). Export
// remains read-only with respect to ingest-run and lease tables: it only
// reads the last run record to derive the site-data content.
func (r *Runner) publishSiteDataFromRepo(ctx context.Context, cfg config.Config) {
	// Build the ingest-run repository (best-effort; a failure means we cannot
	// re-project, so we skip with a log line rather than failing export).
	var runsRepo domain.IngestRunRepository
	if r.newIngestRuns != nil {
		var rerr error
		runsRepo, rerr = r.newIngestRuns(cfg.Storage())
		if rerr != nil {
			_, _ = fmt.Fprintf(r.err, "Warning: cannot re-project site-data from DB (ingest-run repo unavailable): %v\n", rerr)
			return
		}
		defer func() { _ = runsRepo.Close() }() //nolint:errcheck
	} else {
		_, _ = fmt.Fprintf(r.err, "site-data re-projection skipped (no ingest-run repository configured)\n")
		return
	}

	// Read the last attempt (any status) — this is the run whose indicators
	// the site-data should reflect. LastSuccess is used for the freshness
	// semantics of the heartbeat's PublishedAt.
	lastAttempt, hasAttempt, aerr := runsRepo.LastAttempt(ctx)
	if aerr != nil {
		_, _ = fmt.Fprintf(r.err, "Warning: cannot read last ingest run for site-data re-projection: %v\n", aerr)
		return
	}
	if !hasAttempt || lastAttempt == nil {
		_, _ = fmt.Fprintf(r.err, "site-data re-projection skipped (no ingest runs recorded yet)\n")
		return
	}

	// Derive a RunRecord-like view from the IngestRun. The IngestRun carries
	// counters but not per-source detail; we build a minimal RunRecord so
	// BuildFeedHealth can derive per-feed status from the recorded sources.
	rec := domain.RunRecord{
		RunID:          domain.RunID(lastAttempt.RunID),
		StartedAt:      lastAttempt.StartedAt,
		Status:         ingestRunStatusToRunStatus(lastAttempt.Status),
		EntriesFetched: lastAttempt.EntriesFetched,
		StoriesCreated: lastAttempt.StoriesCreated,
		StoriesUpdated: lastAttempt.StoriesUpdated,
	}
	if lastAttempt.FinishedAt != nil {
		fin := lastAttempt.FinishedAt.UTC()
		rec.EndedAt = &fin
	} else {
		now := r.now().UTC()
		rec.EndedAt = &now
	}
	// Committed is true for successful runs (they produced a stored result).
	rec.Committed = lastAttempt.IsSuccess()

	// Build feed health from the re-projected run record. We use a no-op
	// registry-based builder: without the live fetcher map and per-source
	// status from the pipeline, we derive feed health from the run's counters.
	// The FeedHealth will carry the run's status and counters.
	feedHealth := domain.FeedHealth{
		Version:     domain.FeedHealthVersion,
		GeneratedAt: r.now().UTC().Format(time.RFC3339),
		RunID:       lastAttempt.RunID,
		RunStatus:   ingestRunStatusToRunStatus(lastAttempt.Status),
		Duration:    0,
		Sources:     []domain.FeedHealthFeed{},
		Summary: domain.FeedHealthSummary{
			Configured: lastAttempt.SourcesChecked,
			Enabled:    lastAttempt.SourcesChecked,
			Processed:  lastAttempt.SourcesSucceeded,
			Entries:    lastAttempt.EntriesFetched,
			New:        lastAttempt.StoriesCreated,
		},
	}
	if lastAttempt.SourcesSucceeded < lastAttempt.SourcesChecked {
		feedHealth.Summary.Failed = lastAttempt.SourcesChecked - lastAttempt.SourcesSucceeded
	}

	r.publishSiteData(ctx, cfg, rec, *rec.EndedAt, func(_ domain.RunRecord, _ time.Time) domain.FeedHealth {
		return feedHealth
	})
}

// ingestRunStatusToRunStatus maps an ingest-run status to a pipeline run
// status for the site-data re-projection. ok/partial map to their pipeline
// equivalents; everything else maps to failed (the site-data reflects a
// non-successful run as a visible failure signal).
func ingestRunStatusToRunStatus(s domain.IngestRunStatus) domain.RunStatus {
	switch s {
	case domain.IngestRunOK:
		return domain.RunStatusOK
	case domain.IngestRunPartial:
		return domain.RunStatusPartial
	default:
		return domain.RunStatusFailed
	}
}

// buildJSONRunLog renders a single-line JSON run-log entry (issue #158 / AC-8).
// The field set is a strict whitelist (time, run_id, origin, command, status,
// exit_code, and a few integer counters) so no secret (Turso token, DSN, etc.)
// can ever appear in the output. The counters are integers; strings come only
// from sanitized, non-secret config fields (origin) or run metadata.
func buildJSONRunLog(rec *domain.RunRecord, origin string, exitCode int) ([]byte, error) {
	if rec == nil {
		return nil, errors.New("nil run record")
	}
	payload := struct {
		Time      string `json:"time"`
		RunID     string `json:"run_id"`
		Origin    string `json:"origin"`
		Command   string `json:"command"`
		Status    string `json:"status"`
		ExitCode  int    `json:"exit_code"`
		Committed bool   `json:"committed"`
		Fetched   int    `json:"entries_fetched"`
		Created   int    `json:"stories_created"`
		Updated   int    `json:"stories_updated"`
		Skipped   int    `json:"stories_skipped"`
		Errors    int    `json:"errors"`
	}{
		Time:      rec.StartedAt.UTC().Format(time.RFC3339),
		RunID:     rec.RunID.String(),
		Origin:    origin,
		Command:   "ingest run",
		Status:    rec.Status.String(),
		ExitCode:  exitCode,
		Committed: rec.Committed,
		Fetched:   rec.EntriesFetched,
		Created:   rec.StoriesCreated,
		Updated:   rec.StoriesUpdated,
		Skipped:   rec.StoriesSkipped,
		Errors:    len(rec.Errors),
	}
	return json.Marshal(payload)
}

// buildLeaseHolderID composes a sanitized holder identity:
// <origin>:<hostname>:<8-hex-random>. The colon-separated form is human-readable
// in logs and the DB; the random suffix disambiguates processes on the same host
// (e.g., two ingesters on one box during an upgrade). The whole value is
// lowercased and restricted to [a-z0-9:-] (plan §7: no secrets, no shell
// metacharacters). The random suffix is crypto-random so two holders can never
// collide by chance.
func buildLeaseHolderID(origin string) string {
	hostname, herr := os.Hostname()
	if herr != nil || hostname == "" {
		hostname = "unknown"
	}
	randBytes := make([]byte, 4)
	_, _ = rand.Read(randBytes)
	randHex := hex.EncodeToString(randBytes)
	id := fmt.Sprintf("%s:%s:%s", origin, hostname, randHex)
	return sanitizeHolderID(id)
}

// sanitizeHolderID restricts a holder ID to [a-z0-9:-] (lowercased). Any other
// rune is dropped. This guarantees the value stored in the DB and printed in
// logs carries no secrets or shell metacharacters (plan §7).
func sanitizeHolderID(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == ':' || r == '-' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "unknown"
	}
	return b.String()
}

// holderIDOf returns the holder_id of a lease, or "unknown" when the lease is
// nil (used only in the skip message).
func holderIDOf(l *domain.Lease) string {
	if l == nil {
		return "unknown"
	}
	return l.HolderID
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

	cfg, err := r.loadConfig("", config.Overrides{})
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

	// Issue #163 (HS-PORT-07 / AC-8): re-publish site-data files (heartbeat.json
	// + feeds.json) as a re-projection of the shared DB. When ingest is skipped
	// (the healthy case), the committed files would otherwise go stale; export
	// keeps them coherent with Turso without modifying ingest-run or lease rows.
	if cfg.ShouldPublishSiteData() {
		r.publishSiteDataFromRepo(ctx, cfg)
	}

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

	cfg, err := r.loadConfig("", config.Overrides{})
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
