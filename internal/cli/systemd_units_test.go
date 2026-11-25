package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSystemdServiceUnit (issue #162 AC-1) parses the committed systemd
// service unit text and asserts the structural invariants that make a
// one-shot, non-root, journal-logged ingest safe to run unattended. It stands
// in for `systemd-analyze verify` (no systemd host on check runs).
func TestSystemdServiceUnit(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "deploy", "systemd", "huddlescope-ingest.service"))
	require.NoError(t, err, "service unit must exist at deploy/systemd/")
	content := string(data)

	// One-shot, non-daemon.
	assert.Contains(t, content, "Type=oneshot", "service must be Type=oneshot (not a daemon)")

	// Non-root service user/group.
	var userLine, groupLine string
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "User=") {
			userLine = trimmed
		}
		if strings.HasPrefix(trimmed, "Group=") {
			groupLine = trimmed
		}
	}
	require.NotEmpty(t, userLine, "service must set User=")
	require.NotEmpty(t, groupLine, "service must set Group=")
	userValue := strings.TrimPrefix(userLine, "User=")
	groupValue := strings.TrimPrefix(groupLine, "Group=")
	assert.NotEqual(t, "root", userValue, "service must not run as root")
	assert.NotEqual(t, "0", userValue, "service must not run as uid 0")
	assert.NotEqual(t, "root", groupValue, "service must not run in group root")
	assert.NotEqual(t, "0", groupValue, "service must not run in gid 0")

	// Env file + exact exec line (env tier of the precedence chain).
	assert.Contains(t, content, "EnvironmentFile=/etc/huddlescope/huddlescope.env",
		"service must load /etc/huddlescope/huddlescope.env")
	assert.Contains(t, content, "ExecStart=/usr/local/bin/huddlescope ingest run --mode primary",
		"ExecStart must invoke the installed binary in primary mode")

	// Working directory is absolute and owned by the service user.
	assert.Contains(t, content, "WorkingDirectory=/var/lib/huddlescope",
		"WorkingDirectory must be /var/lib/huddlescope")

	// Timeout bound so a hung run is killed before the next 30-min tick.
	assert.Regexp(t, regexp.MustCompile(`(?m)^TimeoutStartSec=\d+$`), content,
		"service must set a numeric TimeoutStartSec")

	// Journal logging.
	assert.Contains(t, content, "StandardOutput=journal", "service must log stdout to the journal")
	assert.Contains(t, content, "StandardError=journal", "service must log stderr to the journal")

	// Moderate hardening (non-root; binary needs no privileges).
	for _, key := range []string{
		"NoNewPrivileges=yes",
		"ProtectSystem=full",
		"ProtectHome=read-only",
		"PrivateTmp=yes",
		"ProtectKernelTunables=yes",
		"ProtectKernelModules=yes",
		"ProtectKernelLogs=yes",
		"ProtectControlGroups=yes",
		"LockPersonality=yes",
		"RestrictRealtime=yes",
	} {
		assert.Contains(t, content, key, "service must set "+key)
	}

	// Negative gates (strip full-line comments first so a comment that merely
	// mentions a forbidden pattern does not trip the gate).
	stripped := stripFullLineComments(content)
	assert.NotContains(t, stripped, "User=root", "service must not run as root")
	assert.NotContains(t, stripped, "User=0", "service must not run as uid 0")
	assert.NotRegexp(t, regexp.MustCompile(`(?im)while\s+true`), stripped,
		"service must not contain an infinite loop")
	assert.NotRegexp(t, regexp.MustCompile(`(?m)^ExecStart=.*--daemon`), stripped,
		"service must not run in a daemon/foreground loop mode")
	// No inline secret: no uncommented unit line sets a TURSO_* or *TOKEN value
	// (secrets come only from EnvironmentFile, never inlined).
	for _, line := range strings.Split(stripped, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if regexp.MustCompile(`(?i)^(TURSO_[A-Z_]+|[A-Z_]*TOKEN)`).MatchString(trimmed) {
			t.Errorf("service must not inline a TURSO_*/token value on %q (secrets come from EnvironmentFile)", trimmed)
		}
	}
}

// TestSystemdTimerUnit (issue #162 AC-2) asserts the timer fires every 30
// minutes, performs a bounded catch-up after boot, and is wired to the
// service. Overlap control is structural (documented in the unit): systemd
// never re-triggers an active oneshot + TimeoutStartSec bound + Turso lease.
func TestSystemdTimerUnit(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "deploy", "systemd", "huddlescope-ingest.timer"))
	require.NoError(t, err, "timer unit must exist at deploy/systemd/")
	content := string(data)

	assert.Contains(t, content, "OnCalendar=*-*-* *:00,30:00",
		"timer must fire every 30 minutes (*:00,30:00)")
	assert.Contains(t, content, "Persistent=true",
		"timer must be Persistent=true (bounded catch-up after boot)")
	assert.Contains(t, content, "Unit=huddlescope-ingest.service",
		"timer must target huddlescope-ingest.service")
	assert.Contains(t, content, "WantedBy=timers.target",
		"timer must be WantedBy=timers.target")

	// Negative: no in-container/in-unit loop.
	stripped := stripFullLineComments(content)
	assert.NotRegexp(t, regexp.MustCompile(`(?im)while\s+true`), stripped,
		"timer must not contain an infinite loop")
}

// TestDeployEnvExample (issue #162 AC-3) asserts the committed env example is
// secret-free (placeholder-only TURSO_*) and carries the production keys,
// absolute paths, and 0600/ownership notes.
func TestDeployEnvExample(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "deploy", "examples", "huddlescope.env.example"))
	require.NoError(t, err, "env example must exist at deploy/examples/")
	content := string(data)

	// Required production keys present (uncommented).
	for _, key := range []string{
		"HSC_ENV=production",
		"HSC_STORAGE_BACKEND=turso",
		"HSC_PUBLISH_SITE_DATA=auto",
		"HSC_SOURCES_FILE=/etc/huddlescope/sources.yaml",
		"HSC_TEAMS_FILE=/etc/huddlescope/teams.yaml",
	} {
		assert.Contains(t, content, key, "env example must set "+key)
	}

	// No uncommented secret value: every TURSO_* / TOKEN line must be
	// commented out or a placeholder (no real DSN/token committed).
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		isComment := strings.HasPrefix(trimmed, "#")
		matchesSecretKey := regexp.MustCompile(`(?i)^(TURSO_[A-Z_]+|[A-Z_]*TOKEN)`).MatchString(trimmed)
		if matchesSecretKey && !isComment {
			t.Errorf("env example must not set a real TURSO_*/token value uncommented: %q", trimmed)
		}
	}

	// 0600 + service-user ownership notes present.
	assert.Regexp(t, regexp.MustCompile(`(?i)0600`), content,
		"env example must note 0600 permissions")
	assert.Regexp(t, regexp.MustCompile(`(?i)huddlescope:huddlescope`), content,
		"env example must note the service-user ownership")
}

// TestComposeFile (issue #162 AC-7) asserts the optional Docker compose path
// is a one-shot, non-root, externally-scheduled run: no in-container loop, no
// always-restart, env supplied via an operator-provided (gitignored) file,
// and no committed secret file referenced.
func TestComposeFile(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "compose.yaml"))
	require.NoError(t, err, "compose.yaml must exist at repo root")
	content := string(data)

	// One-shot, never auto-restarted.
	assert.Contains(t, content, `restart: "no"`, "compose must set restart: \"no\" (one-shot)")

	// Env via an operator-provided file.
	assert.Contains(t, content, "env_file:", "compose must use env_file")

	// Non-root user.
	assert.Contains(t, content, "user: \"65534:65534\"", "compose must run as a non-root user")

	// One-shot ingest command in primary mode.
	assert.Contains(t, content, `command: ["ingest", "run", "--mode", "primary"]`,
		"compose command must be ingest run --mode primary")

	// Negative gates (strip comments so prose mentioning a forbidden pattern
	// does not trip the gate).
	stripped := stripFullLineComments(content)
	assert.NotRegexp(t, regexp.MustCompile(`(?im)while\s+true`), stripped,
		"compose must not contain an infinite loop")
	assert.NotRegexp(t, regexp.MustCompile(`(?im)restart:\s*["']?(always|unless-stopped|on-failure)`), stripped,
		"compose must not auto-restart (one-shot only)")
	assert.NotContains(t, stripped, "deploy/examples/huddlescope.env.example",
		"compose must not reference the committed example as env_file (no secret path in repo)")
}
