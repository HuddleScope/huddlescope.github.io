package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLaunchdAgentPlist (issue #202 AC-9) parses the committed launchd
// LaunchAgent plist and asserts the structural invariants that make a
// one-shot, non-root, env-file-fed ingest safe to run unattended on macOS.
// It stands in for `plutil -lint` / launchd load on check runs (no macOS on
// CI), same philosophy as the systemd unit guard test.
func TestLaunchdAgentPlist(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "deploy", "launchd", "com.huddlescope.ingest.plist"))
	require.NoError(t, err, "plist must exist at deploy/launchd/")
	content := string(data)

	// Well-formed plist envelope.
	assert.Contains(t, content, "<plist version=", "plist must declare its plist version")
	assert.Contains(t, content, "</plist>", "plist must be closed")
	assert.Contains(t, content, "<dict>", "plist root must be a dict")
	assert.Contains(t, content, "</dict>", "plist root dict must be closed")

	// Stable label (what launchctl list / unload / load address).
	assert.Contains(t, content, "<string>com.huddlescope.ingest</string>",
		"plist must use the com.huddlescope.ingest label")

	// One-shot ingest command in primary mode (mirrors the systemd ExecStart),
	// loaded via a sh -c wrapper that sources the 0600 env file first (launchd
	// has no EnvironmentFile; secrets come only from the env file, never
	// inlined).
	require.Contains(t, content, "ingest run --mode primary",
		"plist must run ingest in primary mode")
	assert.Contains(t, content, "set -a", "plist wrapper must export the env file (set -a)")
	assert.Contains(t, content, ". /opt/huddlescope/huddlescope.env",
		"plist wrapper must source /opt/huddlescope/huddlescope.env")
	assert.Contains(t, content, "exec /opt/huddlescope/huddlescope",
		"plist wrapper must exec the installed binary (not run a shell loop)")

	// Working directory is absolute.
	assert.Regexp(t, regexp.MustCompile(`<key>WorkingDirectory</key>\s*<string>/[^<]+</string>`), content,
		"WorkingDirectory must be an absolute path")

	// 30-minute cadence mirroring the systemd timer (OnCalendar *:00,30:00).
	assert.Contains(t, content, "<key>StartInterval</key>", "plist must set StartInterval")
	assert.Contains(t, content, "<integer>1800</integer>",
		"StartInterval must be 1800 (30 min, mirroring the systemd timer)")

	// One-shot semantics: no auto-restart, no run-at-load loop.
	assert.Contains(t, content, "<key>KeepAlive</key>", "plist must set KeepAlive")
	assert.Contains(t, content, "<key>RunAtLoad</key>", "plist must set RunAtLoad")
	// KeepAlive and RunAtLoad must both be false (one-shot, not a daemon).
	assert.Regexp(t, regexp.MustCompile(`(?s)<key>KeepAlive</key>\s*<false/>`), content,
		"KeepAlive must be false (one-shot, not a daemon)")
	assert.Regexp(t, regexp.MustCompile(`(?s)<key>RunAtLoad</key>\s*<false/>`), content,
		"RunAtLoad must be false (no auto-start loop)")

	// Logs to absolute paths under /opt/huddlescope/logs/.
	assert.Contains(t, content, "<key>StandardOutPath</key>", "plist must set StandardOutPath")
	assert.Contains(t, content, "<key>StandardErrorPath</key>", "plist must set StandardErrorPath")
	assert.Contains(t, content, "/opt/huddlescope/logs/",
		"plist log paths must live under /opt/huddlescope/logs/")

	// Negative gates: no inlined secrets (launchd inlines only via
	// EnvironmentVariables / ProgramArguments — the env file is the single
	// secret source, matching the systemd EnvironmentFile posture).
	stripped := stripFullLineComments(content)
	assert.NotContains(t, stripped, "<key>EnvironmentVariables</key>",
		"plist must not inline EnvironmentVariables (secrets come only from the env file)")
	assert.NotRegexp(t, regexp.MustCompile(`(?i)(TURSO_[A-Z_]*|[A-Z_]*TOKEN)\s*[=<]`), stripped,
		"plist must not inline any TURSO_*/token value")
	assert.NotRegexp(t, regexp.MustCompile(`(?im)while\s+true`), stripped,
		"plist must not contain an infinite loop")
	assert.NotRegexp(t, regexp.MustCompile(`(?i)--daemon`), stripped,
		"plist must not run in a daemon/foreground loop mode")
	// No root: the agent runs as the current user (a LaunchAgent, never a
	// LaunchDaemon) and the wrapper must not invoke sudo.
	assert.NotContains(t, stripped, "sudo", "plist wrapper must not escalate to root")
}

// TestLaunchdPlistProgramArgumentsOrder asserts the ProgramArguments array
// carries the sh -c wrapper in the exact expected order (interpreter, -c,
// program string) so launchd execs the wrapper and not a bare shell.
func TestLaunchdPlistProgramArgumentsOrder(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "deploy", "launchd", "com.huddlescope.ingest.plist"))
	require.NoError(t, err)
	content := string(data)

	// Extract the ProgramArguments array body.
	require.Regexp(t, regexp.MustCompile(`(?s)<key>ProgramArguments</key>\s*<array>(.*?)</array>`), content,
		"plist must have a ProgramArguments array")
	m := regexp.MustCompile(`(?s)<key>ProgramArguments</key>\s*<array>(.*?)</array>`).FindStringSubmatch(content)
	require.NotNil(t, m)
	body := m[1]

	strings_ := regexp.MustCompile(`<string>(.*?)</string>`).FindAllStringSubmatch(body, -1)
	require.Equal(t, 3, len(strings_), "ProgramArguments must have exactly 3 entries (sh, -c, program)")
	require.Equal(t, "/bin/sh", strings_[0][1], "first ProgramArguments entry must be /bin/sh")
	require.Equal(t, "-c", strings_[1][1], "second ProgramArguments entry must be -c")
	require.Contains(t, strings_[2][1], "ingest run --mode primary",
		"program string must invoke ingest run --mode primary")
}
