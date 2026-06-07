package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/anstrom/scanorama/internal/db"
)

// Defaults for `scanorama setup`. They mirror the packaged systemd layout: the
// daemon connects as the scanorama role over the local PostgreSQL socket using
// peer authentication, so the bootstrap creates a matching role and database.
const (
	defaultSocketDir     = "/var/run/postgresql"
	defaultSuperuser     = "postgres"
	defaultMaintenanceDB = "postgres"
)

var (
	setupRole          string
	setupDBName        string
	setupHost          string
	setupPort          int
	setupSuperuser     string
	setupMaintenanceDB string
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Bootstrap the local PostgreSQL role and database",
	Long: `Create the PostgreSQL role and database the daemon needs, using peer
authentication over the local socket so no password is stored anywhere.

Run it as the postgres superuser so it can create the role and database:

  sudo -u postgres scanorama setup

It is idempotent — re-running it when the role and database already exist does
nothing. The package postinstall invokes it automatically on a fresh install.`,
	SilenceUsage: true,
	RunE:         runSetup,
}

func runSetup(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	// Privileged connection to the maintenance database as the superuser. Peer
	// auth keys on the OS user, so this must run as the superuser's OS account.
	conn, err := db.Connect(ctx, &db.Config{
		Host:     setupHost,
		Port:     setupPort,
		Database: setupMaintenanceDB,
		Username: setupSuperuser,
		SSLMode:  "disable",
	})
	if err != nil {
		return fmt.Errorf("connecting to PostgreSQL as %q over %s: %w", setupSuperuser, setupHost, err)
	}
	defer func() { _ = conn.Close() }()

	res, err := db.EnsureRoleAndDatabase(ctx, conn, setupRole, setupDBName)
	if err != nil {
		return err
	}

	reportSetup(cmd, res)
	return nil
}

// reportSetup prints what changed so a fresh bootstrap is distinguishable from a
// no-op re-run.
func reportSetup(cmd *cobra.Command, res db.BootstrapResult) {
	var msg string
	switch {
	case res.RoleCreated && res.DatabaseCreated:
		msg = fmt.Sprintf("Created role %q and database %q.\n", setupRole, setupDBName)
	case res.DatabaseCreated:
		msg = fmt.Sprintf("Role %q already existed; created database %q.\n", setupRole, setupDBName)
	case res.RoleCreated:
		msg = fmt.Sprintf("Created role %q; database %q already existed.\n", setupRole, setupDBName)
	default:
		msg = fmt.Sprintf("Role %q and database %q already exist; nothing to do.\n", setupRole, setupDBName)
	}
	_, _ = fmt.Fprint(cmd.OutOrStdout(), msg)
}

func init() {
	rootCmd.AddCommand(setupCmd)

	setupCmd.Flags().StringVar(&setupRole, "role", appName, "login role to create for the daemon")
	setupCmd.Flags().StringVar(&setupDBName, "database", appName, "database to create, owned by the role")
	setupCmd.Flags().StringVar(&setupHost, "host", defaultSocketDir,
		"PostgreSQL host or socket directory to connect to")
	setupCmd.Flags().IntVar(&setupPort, "port", defaultDatabasePort, "PostgreSQL port")
	setupCmd.Flags().StringVar(&setupSuperuser, "superuser", defaultSuperuser,
		"superuser role used to create the role and database")
	setupCmd.Flags().StringVar(&setupMaintenanceDB, "maintenance-db", defaultMaintenanceDB,
		"existing database to connect to while bootstrapping")
}
