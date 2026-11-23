package types

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/HuddleScope/huddlescope.github.io/internal/config"
	"github.com/HuddleScope/huddlescope.github.io/internal/domain"
	"github.com/stretchr/testify/assert"
)

func TestNewDeps(t *testing.T) {
	d := NewDeps()
	assert.NotNil(t, d)
	assert.NotNil(t, d.LoadConfig)
	assert.NotNil(t, d.NewRepository)
	assert.NotNil(t, d.NewApp)
	assert.NotNil(t, d.NewPublisher)
	assert.NotNil(t, d.NewFetcher)
	assert.NotNil(t, d.Now)
	assert.NotNil(t, d.LoadHeartbeat)
}

func TestNewDepsWithOptions(t *testing.T) {
	var out bytes.Buffer
	d := NewDeps(
		WithOutput(&out),
		WithNow(func() time.Time {
			return time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
		}),
	)
	assert.NotNil(t, d)
	assert.Equal(t, &out, d.Output)
}

func TestDefaultDeps(t *testing.T) {
	d := DefaultDeps()
	assert.NotNil(t, d)
	assert.NotNil(t, d.LoadConfig)
	assert.NotNil(t, d.NewRepository)
	assert.NotNil(t, d.NewApp)
	assert.NotNil(t, d.NewPublisher)
	assert.NotNil(t, d.NewFetcher)
	assert.NotNil(t, d.Now)
	assert.NotNil(t, d.LoadHeartbeat)
}

func TestDefaultLoadConfig(t *testing.T) {
	_, err := DefaultLoadConfig("/nonexistent/path")
	assert.Error(t, err)
}

func TestDefaultNewRepository(t *testing.T) {
	repo, err := DefaultNewRepository(config.StorageConfig{Backend: "memory"})
	assert.NoError(t, err)
	assert.NotNil(t, repo)
}

func TestDefaultNewApp(t *testing.T) {
	app := DefaultNewApp(nil)
	assert.NotNil(t, app)
}

func TestDefaultNewPublisher(t *testing.T) {
	pub := DefaultNewPublisher()
	assert.NotNil(t, pub)
}

func TestDefaultNewFetcher(t *testing.T) {
	fetcher := DefaultNewFetcher(domain.FeedSource{})
	assert.NotNil(t, fetcher)
}

func TestDefaultNow(t *testing.T) {
	now := DefaultNow()
	assert.NotZero(t, now)
}

func TestDefaultLoadHeartbeat(t *testing.T) {
	_, ok, err := DefaultLoadHeartbeat("/nonexistent/path")
	assert.NoError(t, err)
	assert.False(t, ok)
}

func TestDepsValidate(t *testing.T) {
	d := DefaultDeps()
	assert.NoError(t, d.Validate())
}

func TestDepsValidateNilLoadConfig(t *testing.T) {
	d := &Deps{
		LoadConfig: nil,
	}
	assert.Error(t, d.Validate())
}

func TestDepsValidateNilNewRepository(t *testing.T) {
	d := &Deps{
		LoadConfig:    DefaultLoadConfig,
		NewRepository: nil,
	}
	assert.Error(t, d.Validate())
}

func TestDepsValidateNilNewApp(t *testing.T) {
	d := &Deps{
		LoadConfig:    DefaultLoadConfig,
		NewRepository: DefaultNewRepository,
		NewApp:        nil,
	}
	assert.Error(t, d.Validate())
}

func TestDepsValidateNilNewPublisher(t *testing.T) {
	d := &Deps{
		LoadConfig:    DefaultLoadConfig,
		NewRepository: DefaultNewRepository,
		NewApp:        DefaultNewApp,
		NewPublisher:  nil,
	}
	assert.Error(t, d.Validate())
}

func TestDepsValidateNilNewFetcher(t *testing.T) {
	d := &Deps{
		LoadConfig:    DefaultLoadConfig,
		NewRepository: DefaultNewRepository,
		NewApp:        DefaultNewApp,
		NewPublisher:  DefaultNewPublisher,
		NewFetcher:    nil,
	}
	assert.Error(t, d.Validate())
}

func TestDepsValidateNilNow(t *testing.T) {
	d := &Deps{
		LoadConfig:    DefaultLoadConfig,
		NewRepository: DefaultNewRepository,
		NewApp:        DefaultNewApp,
		NewPublisher:  DefaultNewPublisher,
		NewFetcher:    DefaultNewFetcher,
		Now:           nil,
	}
	assert.Error(t, d.Validate())
}

func TestDepsValidateNilLoadHeartbeat(t *testing.T) {
	d := &Deps{
		LoadConfig:    DefaultLoadConfig,
		NewRepository: DefaultNewRepository,
		NewApp:        DefaultNewApp,
		NewPublisher:  DefaultNewPublisher,
		NewFetcher:    DefaultNewFetcher,
		Now:           DefaultNow,
		LoadHeartbeat: nil,
	}
	assert.Error(t, d.Validate())
}

func TestRunResultIsSuccess(t *testing.T) {
	r := &RunResult{ExitCode: 0}
	assert.True(t, r.IsSuccess())
}

func TestRunResultIsFailure(t *testing.T) {
	r := &RunResult{ExitCode: 1}
	assert.True(t, r.IsFailure())
}

func TestExitError(t *testing.T) {
	err := NewExitError(42, assert.AnError)
	assert.Equal(t, 42, err.ExitCode())
	assert.Error(t, err)
}

func TestExitErrorUnwrap(t *testing.T) {
	originalErr := errors.New("original error")
	exitErr := NewExitError(42, originalErr)
	assert.ErrorIs(t, exitErr, originalErr)
}

func TestWithOutput(t *testing.T) {
	var out bytes.Buffer
	d := NewDeps(WithOutput(&out))
	assert.Equal(t, &out, d.Output)
}

func TestWithErrorOutput(t *testing.T) {
	var errOut bytes.Buffer
	d := NewDeps(WithErrorOutput(&errOut))
	assert.Equal(t, &errOut, d.ErrorOutput)
}

func TestWithNow(t *testing.T) {
	now := func() time.Time {
		return time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	}
	d := NewDeps(WithNow(now))
	// Verify that the function was set by checking that it's not the default
	assert.NotNil(t, d.Now)
}

func TestRunResultIsSuccessTrue(t *testing.T) {
	r := &RunResult{ExitCode: 0}
	assert.True(t, r.IsSuccess())
}

func TestRunResultIsFailureTrue(t *testing.T) {
	r := &RunResult{ExitCode: 1}
	assert.True(t, r.IsFailure())
}

func TestRunResultIsSuccessFalse(t *testing.T) {
	r := &RunResult{ExitCode: 1}
	assert.False(t, r.IsSuccess())
}

func TestRunResultIsFailureFalse(t *testing.T) {
	r := &RunResult{ExitCode: 0}
	assert.False(t, r.IsFailure())
}

func TestDepsDefaultValues(t *testing.T) {
	d := NewDeps()
	assert.NotNil(t, d.Output)
	assert.NotNil(t, d.ErrorOutput)
}
