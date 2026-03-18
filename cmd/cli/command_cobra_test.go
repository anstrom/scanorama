// Package cli provides Cobra command-level tests for the CLI.
//
// These are white-box (internal) tests: they live in package cli and access
// package-level variables such as rootCmd directly.
//
// Design notes
// ─────────────
//
//   - Output is captured via Cobra's SetOut / SetErr writers (backed by
//     bytes.Buffer) rather than redirecting os.Stdout / os.Stderr.  Cobra
//     routes all of its own output (help text, version string, error messages)
//     through the command's OutOrStdout() / OutOrStderr() writers, so this is
//     both correct and race-safe.
//
//   - rootCmd is a package-level singleton whose flag state persists across
//     Execute() calls.  After a call such as executeCommand("--help"), the
//     --help boolean flag remains true.  On the very next Execute() call Cobra
//     sees the flag already set and short-circuits into the help handler again,
//     causing tests to interfere with each other.  resetFlags() walks the
//     entire command tree and resets every changed flag back to its default
//     before each test concludes.
package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── helpers ─────────────────────────────────────────────────────────────────

// resetFlagSet resets every changed flag in fs back to its declared default.
func resetFlagSet(fs *pflag.FlagSet) {
	if fs == nil {
		return
	}
	fs.VisitAll(func(f *pflag.Flag) {
		if f.Changed {
			_ = f.Value.Set(f.DefValue) // best-effort; error intentionally ignored
			f.Changed = false
		}
	})
}

// resetFlags walks rootCmd and all of its sub-commands (up to three levels
// deep, which covers every command registered in this package) and resets
// every flag that was changed during the previous Execute() call.
//
// rootCmd.Commands() is available because this file is in the same package as
// root.go where rootCmd is declared as *cobra.Command.
func resetFlags() {
	resetFlagSet(rootCmd.Flags())
	resetFlagSet(rootCmd.PersistentFlags())

	for _, sub := range rootCmd.Commands() {
		resetFlagSet(sub.Flags())
		resetFlagSet(sub.PersistentFlags())

		for _, subsub := range sub.Commands() {
			resetFlagSet(subsub.Flags())
			resetFlagSet(subsub.PersistentFlags())

			for _, subsubsub := range subsub.Commands() {
				resetFlagSet(subsubsub.Flags())
				resetFlagSet(subsubsub.PersistentFlags())
			}
		}
	}
}

// executeCommand runs rootCmd with the supplied args and returns the captured
// stdout, stderr, and any error returned by Execute().
//
// After every call the full command tree is walked to reset changed flag
// values back to their defaults, and the args slice is cleared, so that one
// test cannot pollute the next.
func executeCommand(args ...string) (stdout, stderr string, err error) {
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)

	rootCmd.SetOut(outBuf)
	rootCmd.SetErr(errBuf)
	rootCmd.SetArgs(args)

	err = rootCmd.Execute()

	// Reset all changed flags before returning so subsequent calls start clean.
	resetFlags()
	rootCmd.SetArgs([]string{})
	rootCmd.SetOut(nil)
	rootCmd.SetErr(nil)

	return outBuf.String(), errBuf.String(), err
}

// combined joins stdout and stderr into one string, which is handy for
// assertions that do not care which stream a particular message was written to
// (Cobra writes some messages to stdout and others to stderr depending on the
// command and the error type).
func combined(out, errOut string) string {
	return out + errOut
}

// ─── 1. Root command ─────────────────────────────────────────────────────────

func TestRootCommandHelp(t *testing.T) {
	out, errOut, err := executeCommand("--help")

	require.NoError(t, err)
	all := combined(out, errOut)
	assert.Contains(t, all, "scanorama",
		"help output should mention the binary name")
	assert.Contains(t, all, "Available Commands",
		"help output should contain the Available Commands section")
}

func TestRootCommandVersion(t *testing.T) {
	out, errOut, err := executeCommand("--version")

	require.NoError(t, err)
	all := combined(out, errOut)
	// The default build-time version string is "dev"; getVersion() formats it
	// as "dev (commit: none, built: unknown)".
	assert.Contains(t, all, "dev",
		"version output should contain the default version string")
}

// ─── 2. Scan command ─────────────────────────────────────────────────────────

func TestScanCommandHelp(t *testing.T) {
	out, errOut, err := executeCommand("scan", "--help")

	require.NoError(t, err)
	all := combined(out, errOut)
	assert.Contains(t, all, "--targets",
		"scan help should document the --targets flag")
	assert.Contains(t, all, "--ports",
		"scan help should document the --ports flag")
	assert.Contains(t, all, "--type",
		"scan help should document the --type flag")
	assert.Contains(t, all, "--live-hosts",
		"scan help should document the --live-hosts flag")
}

func TestScanCommandMutuallyExclusiveFlags(t *testing.T) {
	// --targets and --live-hosts are registered as mutually exclusive via
	// scanCmd.MarkFlagsMutuallyExclusive.  Cobra validates this during
	// Execute() and returns a non-nil error without calling os.Exit, so we
	// can assert on it directly.
	_, errOut, err := executeCommand("scan", "--targets", "192.168.1.1", "--live-hosts")

	require.Error(t, err,
		"providing both --targets and --live-hosts must produce an error")

	// Cobra's error message for mutually exclusive flags does not use the
	// phrase "mutually exclusive" verbatim; it says something along the lines
	// of "if any flags in the group [...] are set none of the others can be".
	// We therefore check for the flag names in the error output, which is
	// stable across Cobra versions, rather than matching the prose wording.
	errMsg := err.Error()
	errAll := errMsg + errOut
	assert.True(t,
		strings.Contains(errAll, "targets") && strings.Contains(errAll, "live-hosts"),
		"error should reference both flag names; got err=%q stderr=%q",
		err, errOut,
	)
}

// ─── 3. Hosts command ────────────────────────────────────────────────────────

func TestHostsCommandHelp(t *testing.T) {
	out, errOut, err := executeCommand("hosts", "--help")

	require.NoError(t, err)
	all := combined(out, errOut)
	assert.Contains(t, all, "--status",
		"hosts help should document the --status flag")
	assert.Contains(t, all, "--os",
		"hosts help should document the --os flag")
	assert.Contains(t, all, "--last-seen",
		"hosts help should document the --last-seen flag")
	assert.Contains(t, all, "--show-ignored",
		"hosts help should document the --show-ignored flag")
}

func TestHostsSubcommands(t *testing.T) {
	t.Run("ignore subcommand help", func(t *testing.T) {
		out, errOut, err := executeCommand("hosts", "ignore", "--help")

		require.NoError(t, err)
		all := combined(out, errOut)
		// The Use string is "ignore [IP]".
		assert.Contains(t, all, "[IP]",
			"hosts ignore help should document the [IP] positional argument")
	})

	t.Run("unignore subcommand help", func(t *testing.T) {
		out, errOut, err := executeCommand("hosts", "unignore", "--help")

		require.NoError(t, err)
		all := combined(out, errOut)
		// The Use string is "unignore [IP]".
		assert.Contains(t, all, "[IP]",
			"hosts unignore help should document the [IP] positional argument")
	})
}

// ─── 4. Schedule command ─────────────────────────────────────────────────────

func TestScheduleCommandStructure(t *testing.T) {
	t.Run("schedule help lists subcommands", func(t *testing.T) {
		out, errOut, err := executeCommand("schedule", "--help")

		require.NoError(t, err)
		all := combined(out, errOut)
		assert.Contains(t, all, "add-discovery",
			"schedule help should list the add-discovery subcommand")
		assert.Contains(t, all, "add-scan",
			"schedule help should list the add-scan subcommand")
		assert.Contains(t, all, "list",
			"schedule help should list the list subcommand")
		assert.Contains(t, all, "remove",
			"schedule help should list the remove subcommand")
		assert.Contains(t, all, "show",
			"schedule help should list the show subcommand")
	})

	t.Run("add-discovery help shows positional arg names", func(t *testing.T) {
		out, errOut, err := executeCommand("schedule", "add-discovery", "--help")

		require.NoError(t, err)
		all := combined(out, errOut)
		// The Use string is "add-discovery [name] [cron] [network]".
		assert.Contains(t, all, "[name]",
			"add-discovery help should describe the [name] argument")
		assert.Contains(t, all, "[cron]",
			"add-discovery help should describe the [cron] argument")
		assert.Contains(t, all, "[network]",
			"add-discovery help should describe the [network] argument")
	})

	t.Run("add-scan help shows scan flags", func(t *testing.T) {
		out, errOut, err := executeCommand("schedule", "add-scan", "--help")

		require.NoError(t, err)
		all := combined(out, errOut)
		assert.Contains(t, all, "--targets",
			"add-scan help should document the --targets flag")
		assert.Contains(t, all, "--live-hosts",
			"add-scan help should document the --live-hosts flag")
	})
}

func TestScheduleAddDiscoveryArgValidation(t *testing.T) {
	// scheduleAddDiscoveryCmd is declared with cobra.ExactArgs(3). Cobra
	// validates the argument count before invoking Run and returns a non-nil
	// error from Execute() (without calling os.Exit), so we can assert on it.

	t.Run("no args returns error", func(t *testing.T) {
		_, _, err := executeCommand("schedule", "add-discovery")
		assert.Error(t, err,
			"add-discovery with no args should fail (requires exactly 3 positional args)")
	})

	t.Run("one arg returns error", func(t *testing.T) {
		_, _, err := executeCommand("schedule", "add-discovery", "only-one-arg")
		assert.Error(t, err,
			"add-discovery with one arg should fail (requires exactly 3 positional args)")
	})

	t.Run("two args returns error", func(t *testing.T) {
		_, _, err := executeCommand("schedule", "add-discovery", "name-arg", "cron-arg")
		assert.Error(t, err,
			"add-discovery with two args should fail (requires exactly 3 positional args)")
	})
}

// ─── 5. Daemon command ───────────────────────────────────────────────────────

func TestDaemonCommandStructure(t *testing.T) {
	t.Run("daemon help lists subcommands", func(t *testing.T) {
		out, errOut, err := executeCommand("daemon", "--help")

		require.NoError(t, err)
		all := combined(out, errOut)
		assert.Contains(t, all, "start",
			"daemon help should list the start subcommand")
		assert.Contains(t, all, "stop",
			"daemon help should list the stop subcommand")
		assert.Contains(t, all, "status",
			"daemon help should list the status subcommand")
		assert.Contains(t, all, "restart",
			"daemon help should list the restart subcommand")
	})

	t.Run("daemon start help shows flags", func(t *testing.T) {
		out, errOut, err := executeCommand("daemon", "start", "--help")

		require.NoError(t, err)
		all := combined(out, errOut)
		// --pid-file is a persistent flag on daemonCmd, so it is inherited by
		// all daemon sub-commands and must appear in their help output.
		assert.Contains(t, all, "--pid-file",
			"daemon start help should show the --pid-file persistent flag")
		assert.Contains(t, all, "--port",
			"daemon start help should show the --port flag")
	})

	t.Run("daemon status help returns no error", func(t *testing.T) {
		_, _, err := executeCommand("daemon", "status", "--help")
		require.NoError(t, err)
	})
}

// ─── 6. Networks command ─────────────────────────────────────────────────────

func TestNetworksCommandStructure(t *testing.T) {
	t.Run("networks help lists subcommands", func(t *testing.T) {
		out, errOut, err := executeCommand("networks", "--help")

		require.NoError(t, err)
		all := combined(out, errOut)
		assert.Contains(t, all, "add",
			"networks help should list the add subcommand")
		assert.Contains(t, all, "list",
			"networks help should list the list subcommand")
		assert.Contains(t, all, "remove",
			"networks help should list the remove subcommand")
		assert.Contains(t, all, "show",
			"networks help should list the show subcommand")
		assert.Contains(t, all, "enable",
			"networks help should list the enable subcommand")
		assert.Contains(t, all, "disable",
			"networks help should list the disable subcommand")
		assert.Contains(t, all, "rename",
			"networks help should list the rename subcommand")
		assert.Contains(t, all, "stats",
			"networks help should list the stats subcommand")
		assert.Contains(t, all, "exclusions",
			"networks help should list the exclusions subcommand")
	})

	t.Run("networks add help shows required flags", func(t *testing.T) {
		out, errOut, err := executeCommand("networks", "add", "--help")

		require.NoError(t, err)
		all := combined(out, errOut)
		assert.Contains(t, all, "--name",
			"networks add help should document the --name flag")
		assert.Contains(t, all, "--cidr",
			"networks add help should document the --cidr flag")
		assert.Contains(t, all, "--method",
			"networks add help should document the --method flag")
	})

	t.Run("networks remove with no args returns error", func(t *testing.T) {
		// networksRemoveCmd is declared with cobra.ExactArgs(1).
		_, _, err := executeCommand("networks", "remove")
		assert.Error(t, err,
			"networks remove with no args should fail (requires exactly 1 positional arg)")
	})
}

// ─── 7. Profiles command ─────────────────────────────────────────────────────

func TestProfilesCommandStructure(t *testing.T) {
	t.Run("profiles help lists subcommands", func(t *testing.T) {
		out, errOut, err := executeCommand("profiles", "--help")

		require.NoError(t, err)
		all := combined(out, errOut)
		assert.Contains(t, all, "list",
			"profiles help should list the list subcommand")
		assert.Contains(t, all, "show",
			"profiles help should list the show subcommand")
		assert.Contains(t, all, "test",
			"profiles help should list the test subcommand")
	})

	t.Run("profiles show with no args returns error", func(t *testing.T) {
		// profilesShowCmd is declared with cobra.ExactArgs(1).
		_, _, err := executeCommand("profiles", "show")
		assert.Error(t, err,
			"profiles show with no args should fail (requires exactly 1 positional arg)")
	})

	t.Run("profiles test with no args returns error", func(t *testing.T) {
		// profilesTestCmd is declared with cobra.ExactArgs(1).
		_, _, err := executeCommand("profiles", "test")
		assert.Error(t, err,
			"profiles test with no args should fail (requires exactly 1 positional arg)")
	})
}

// ─── 8. API command ──────────────────────────────────────────────────────────

func TestAPICommandHelp(t *testing.T) {
	out, errOut, err := executeCommand("api", "--help")

	require.NoError(t, err)
	all := combined(out, errOut)
	assert.Contains(t, all, "--host",
		"api help should document the --host flag")
	assert.Contains(t, all, "--port",
		"api help should document the --port flag")
}

// ─── 9. Unknown command ──────────────────────────────────────────────────────

func TestUnknownCommand(t *testing.T) {
	_, _, err := executeCommand("nonexistent-command")
	assert.Error(t, err,
		"an unknown top-level command should return an error")
}
