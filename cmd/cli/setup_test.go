package cli

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/db"
)

func TestReportSetup(t *testing.T) {
	origRole, origDB := setupRole, setupDBName
	t.Cleanup(func() { setupRole, setupDBName = origRole, origDB })
	setupRole = "scanorama"
	setupDBName = "scanorama"

	cases := []struct {
		name   string
		result db.BootstrapResult
		want   string
	}{
		{"both created", db.BootstrapResult{RoleCreated: true, DatabaseCreated: true},
			"Created role \"scanorama\" and database \"scanorama\".\n"},
		{"database only", db.BootstrapResult{DatabaseCreated: true},
			"Role \"scanorama\" already existed; created database \"scanorama\".\n"},
		{"role only", db.BootstrapResult{RoleCreated: true},
			"Created role \"scanorama\"; database \"scanorama\" already existed.\n"},
		{"nothing to do", db.BootstrapResult{},
			"Role \"scanorama\" and database \"scanorama\" already exist; nothing to do.\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			cmd := &cobra.Command{}
			cmd.SetOut(&buf)

			reportSetup(cmd, tc.result)

			assert.Equal(t, tc.want, buf.String())
		})
	}
}

func TestSetupCommandDefaults(t *testing.T) {
	require.NoError(t, setupCmd.Flags().Parse(nil))

	assert.Equal(t, "scanorama", setupCmd.Flags().Lookup("role").DefValue)
	assert.Equal(t, "scanorama", setupCmd.Flags().Lookup("database").DefValue)
	assert.Equal(t, defaultSocketDir, setupCmd.Flags().Lookup("host").DefValue)
	assert.Equal(t, defaultSuperuser, setupCmd.Flags().Lookup("superuser").DefValue)
	assert.Equal(t, defaultMaintenanceDB, setupCmd.Flags().Lookup("maintenance-db").DefValue)
}
