package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		setup   func() (string, func())
		wantErr bool
	}{
		{
			name: "valid yaml config",
			setup: func() (string, func()) {
				content := []byte(`
database:
  host: localhost
  port: 5432
  name: testdb
  user: testuser
  password: testpass
daemon:
  user: nobody
  group: nobody
  pidfile: /var/run/scanorama.pid
`)
				dir := t.TempDir()
				path := filepath.Join(dir, "config.yaml")
				if err := os.WriteFile(path, content, 0644); err != nil {
					t.Fatal(err)
				}
				return path, func() {
					os.Remove(path)
				}
			},
			wantErr: false,
		},
		{
			name: "valid json config",
			setup: func() (string, func()) {
				content := []byte(`{
					"database": {
						"host": "localhost",
						"port": 5432,
						"name": "testdb",
						"user": "testuser",
						"password": "testpass"
					},
					"daemon": {
						"user": "nobody",
						"group": "nobody",
						"pidfile": "/var/run/scanorama.pid"
					}
				}`)
				dir := t.TempDir()
				path := filepath.Join(dir, "config.json")
				if err := os.WriteFile(path, content, 0644); err != nil {
					t.Fatal(err)
				}
				return path, func() {
					os.Remove(path)
				}
			},
			wantErr: false,
		},
		{
			name: "invalid yaml syntax",
			setup: func() (string, func()) {
				content := []byte(`
database:
  host: localhost
  port: invalid
`)
				dir := t.TempDir()
				path := filepath.Join(dir, "config.yaml")
				if err := os.WriteFile(path, content, 0644); err != nil {
					t.Fatal(err)
				}
				return path, func() {
					os.Remove(path)
				}
			},
			wantErr: true,
		},
		{
			name: "invalid json syntax",
			setup: func() (string, func()) {
				content := []byte(`{
					"database": {
						"host": "localhost",
						"port": "invalid"
					},
				}`)
				dir := t.TempDir()
				path := filepath.Join(dir, "config.json")
				if err := os.WriteFile(path, content, 0644); err != nil {
					t.Fatal(err)
				}
				return path, func() {
					os.Remove(path)
				}
			},
			wantErr: true,
		},
		{
			name: "nonexistent file",
			setup: func() (string, func()) {
				return "/nonexistent/config.yaml", func() {}
			},
			wantErr: true,
		},
		{
			name: "unsupported extension",
			setup: func() (string, func()) {
				content := []byte(`config data`)
				dir := t.TempDir()
				path := filepath.Join(dir, "config.txt")
				if err := os.WriteFile(path, content, 0644); err != nil {
					t.Fatal(err)
				}
				return path, func() {
					os.Remove(path)
				}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, cleanup := tt.setup()
			defer cleanup()

			_, err := Load(path)
			if (err != nil) != tt.wantErr {
				t.Errorf("Load() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoadWithEnv(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		setup   func() (string, func())
		check   func(*Config) error
		wantErr bool
	}{
		{
			name: "override database config",
			env: map[string]string{
				"SCANORAMA_DB_HOST":     "env-host",
				"SCANORAMA_DB_PORT":     "5433",
				"SCANORAMA_DB_NAME":     "env-db",
				"SCANORAMA_DB_USER":     "env-user",
				"SCANORAMA_DB_PASSWORD": "env-pass",
			},
			setup: func() (string, func()) {
				content := []byte(`
database:
  host: localhost
  port: 5432
  name: testdb
  user: testuser
  password: testpass
`)
				dir := t.TempDir()
				path := filepath.Join(dir, "config.yaml")
				if err := os.WriteFile(path, content, 0644); err != nil {
					t.Fatal(err)
				}
				return path, func() {
					os.Remove(path)
				}
			},
			check: func(c *Config) error {
				if c.Database.Host != "env-host" ||
					c.Database.Port != 5433 ||
					c.Database.Name != "env-db" ||
					c.Database.User != "env-user" ||
					c.Database.Password != "env-pass" {
					t.Error("environment variables did not override config values")
				}
				return nil
			},
			wantErr: false,
		},
		{
			name: "invalid port in env",
			env: map[string]string{
				"SCANORAMA_DB_PORT": "invalid",
			},
			setup: func() (string, func()) {
				content := []byte(`
database:
  host: localhost
  port: 5432
`)
				dir := t.TempDir()
				path := filepath.Join(dir, "config.yaml")
				if err := os.WriteFile(path, content, 0644); err != nil {
					t.Fatal(err)
				}
				return path, func() {
					os.Remove(path)
				}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original env
			origEnv := make(map[string]string)
			for k := range tt.env {
				if v, ok := os.LookupEnv(k); ok {
					origEnv[k] = v
				}
			}

			// Set test env
			for k, v := range tt.env {
				os.Setenv(k, v)
			}

			// Cleanup env after test
			defer func() {
				for k := range tt.env {
					if orig, ok := origEnv[k]; ok {
						os.Setenv(k, orig)
					} else {
						os.Unsetenv(k)
					}
				}
			}()

			path, cleanup := tt.setup()
			defer cleanup()

			cfg, err := Load(path)
			if (err != nil) != tt.wantErr {
				t.Errorf("Load() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err == nil && tt.check != nil {
				if err := tt.check(cfg); err != nil {
					t.Errorf("check failed: %v", err)
				}
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: &Config{
				Database: DatabaseConfig{
					Host:     "localhost",
					Port:     5432,
					Name:     "testdb",
					User:     "testuser",
					Password: "testpass",
				},
				Daemon: DaemonConfig{
					User:    "nobody",
					Group:   "nobody",
					PIDFile: "/var/run/scanorama.pid",
				},
			},
			wantErr: false,
		},
		{
			name: "missing database host",
			config: &Config{
				Database: DatabaseConfig{
					Port:     5432,
					Name:     "testdb",
					User:     "testuser",
					Password: "testpass",
				},
			},
			wantErr: true,
		},
		{
			name: "invalid database port",
			config: &Config{
				Database: DatabaseConfig{
					Host:     "localhost",
					Port:     0,
					Name:     "testdb",
					User:     "testuser",
					Password: "testpass",
				},
			},
			wantErr: true,
		},
		{
			name: "missing database name",
			config: &Config{
				Database: DatabaseConfig{
					Host:     "localhost",
					Port:     5432,
					User:     "testuser",
					Password: "testpass",
				},
			},
			wantErr: true,
		},
		{
			name: "missing database user",
			config: &Config{
				Database: DatabaseConfig{
					Host:     "localhost",
					Port:     5432,
					Name:     "testdb",
					Password: "testpass",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.config.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("Config.Validate() error = %v, wantErr
