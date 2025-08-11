import React, { useState, useEffect, useCallback } from "react";
import {
  Box,
  Card,
  CardHeader,
  CardContent,
  Button,
  Typography,
  TextField,
  FormControl,
  InputLabel,
  Select,
  MenuItem,
  Switch,
  FormControlLabel,
  Alert,
  CircularProgress,
  Stack,
  Grid,
  Paper,
  Chip,
  List,
  ListItem,
  ListItemText,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Tabs,
  Tab,
  Accordion,
  AccordionSummary,
  AccordionDetails,
} from "@mui/material";
import {
  Save as SaveIcon,
  Refresh as RefreshIcon,
  Security as SecurityIcon,
  Storage as StorageIcon,
  Api as ApiIcon,
  BugReport as LogIcon,
  Info as InfoIcon,
  ExpandMore as ExpandMoreIcon,
  Warning as WarningIcon,
  CheckCircle as CheckIcon,
  Error as ErrorIcon,
} from "@mui/icons-material";
import { SystemConfig, SystemStatus, HealthCheck } from "../types/api";
import { systemAPI } from "../services/api";
import logger, { LogEntry } from "../utils/logger";

interface TabPanelProps {
  children?: React.ReactNode;
  index: number;
  value: number;
}

interface ErrorLogEntry {
  id: string;
  timestamp: string;
  level: string;
  message: string;
  component?: string | undefined;
  error?: Error | undefined;
  metadata?: Record<string, any> | undefined;
}

const TabPanel: React.FC<TabPanelProps> = ({ children, value, index, ...other }) => {
  return (
    <div
      role="tabpanel"
      hidden={value !== index}
      id={`settings-tabpanel-${index}`}
      aria-labelledby={`settings-tab-${index}`}
      {...other}
    >
      {value === index && <Box sx={{ p: 3 }}>{children}</Box>}
    </div>
  );
};

const Settings: React.FC = () => {
  const [activeTab, setActiveTab] = useState(0);
  const [config, setConfig] = useState<SystemConfig | null>(null);
  const [systemStatus, setSystemStatus] = useState<SystemStatus | null>(null);
  const [healthCheck, setHealthCheck] = useState<HealthCheck | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);
  const [confirmDialog, setConfirmDialog] = useState(false);
  const [errorLogs, setErrorLogs] = useState<ErrorLogEntry[]>([]);
  const [logFilter, setLogFilter] = useState<string>("all");

  const loadErrorLogs = useCallback(async () => {
    try {
      const logs = await logger.exportLogs();
      const formattedLogs: ErrorLogEntry[] = logs.map((log: LogEntry) => ({
        id: log.id,
        timestamp: log.timestamp,
        level: log.level.toString(),
        message: log.message,
        component: log.context?.component || undefined,
        error: log.error || undefined,
        metadata: log.metadata || undefined,
      }));
      setErrorLogs(formattedLogs);
    } catch (err) {
      console.error("Error loading error logs:", err);
    }
  }, []);

  useEffect(() => {
    loadSettings();
    loadSystemStatus();
    loadHealthCheck();
    loadErrorLogs();
  }, [loadErrorLogs]);

  const loadSettings = async () => {
    try {
      setLoading(true);
      const response = await systemAPI.getConfig();
      setConfig(response);
      setError(null);
    } catch (err) {
      setError("Failed to load system configuration");
      console.error("Error loading settings:", err);
    } finally {
      setLoading(false);
    }
  };

  const loadSystemStatus = async () => {
    try {
      const response = await systemAPI.getStatus();
      setSystemStatus(response);
    } catch (err) {
      console.error("Error loading system status:", err);
    }
  };

  const loadHealthCheck = async () => {
    try {
      const response = await systemAPI.getHealth();
      setHealthCheck(response);
    } catch (err) {
      console.error("Error loading health check:", err);
    }
  };

  const handleSaveSettings = async () => {
    if (!config) return;

    setConfirmDialog(true);
  };

  const confirmSaveSettings = async () => {
    if (!config) return;

    try {
      setSaving(true);
      await systemAPI.updateConfig(config);
      setSuccess("Settings saved successfully");
      setError(null);
      setConfirmDialog(false);
    } catch (err) {
      setError("Failed to save settings");
      console.error("Error saving settings:", err);
    } finally {
      setSaving(false);
    }
  };

  const handleConfigChange = (section: keyof SystemConfig, field: string, value: any) => {
    if (!config) return;

    setConfig({
      ...config,
      [section]: {
        ...config[section],
        [field]: value,
      },
    });
  };

  const getHealthStatusIcon = (status: string) => {
    switch (status) {
      case "pass":
      case "healthy":
        return <CheckIcon color="success" />;
      case "warn":
      case "warning":
        return <WarningIcon color="warning" />;
      case "fail":
      case "error":
      case "unhealthy":
        return <ErrorIcon color="error" />;
      default:
        return <WarningIcon color="warning" />;
    }
  };

  const getHealthStatusColor = (status: string): "success" | "warning" | "error" => {
    switch (status) {
      case "pass":
      case "healthy":
        return "success";
      case "warn":
      case "warning":
        return "warning";
      case "fail":
      case "error":
      case "unhealthy":
        return "error";
      default:
        return "warning";
    }
  };

  if (loading) {
    return (
      <Box display="flex" justifyContent="center" alignItems="center" minHeight="400px">
        <CircularProgress />
      </Box>
    );
  }

  if (!config) {
    return (
      <Box>
        <Alert severity="error">Failed to load system configuration</Alert>
      </Box>
    );
  }

  return (
    <Box>
      <Box display="flex" justifyContent="space-between" alignItems="center" mb={3}>
        <Typography variant="h4" component="h1">
          Settings
        </Typography>
        <Stack direction="row" spacing={2}>
          <Button startIcon={<RefreshIcon />} onClick={loadSettings} variant="outlined">
            Refresh
          </Button>
          <Button
            startIcon={<SaveIcon />}
            onClick={handleSaveSettings}
            variant="contained"
            disabled={saving}
          >
            {saving ? "Saving..." : "Save Changes"}
          </Button>
        </Stack>
      </Box>

      {error && (
        <Alert severity="error" sx={{ mb: 3 }} onClose={() => setError(null)}>
          {error}
        </Alert>
      )}

      {success && (
        <Alert severity="success" sx={{ mb: 3 }} onClose={() => setSuccess(null)}>
          {success}
        </Alert>
      )}

      <Paper sx={{ width: "100%" }}>
        <Tabs
          value={activeTab}
          onChange={(_, newValue) => setActiveTab(newValue)}
          variant="scrollable"
          scrollButtons="auto"
        >
          <Tab label="API Configuration" icon={<ApiIcon />} />
          <Tab label="Database" icon={<StorageIcon />} />
          <Tab label="Logging" icon={<LogIcon />} />
          <Tab label="Security" icon={<SecurityIcon />} />
          <Tab label="System Status" icon={<InfoIcon />} />
          <Tab label="Error Logs" icon={<ErrorIcon />} />
        </Tabs>

        {/* API Configuration Tab */}
        <TabPanel value={activeTab} index={0}>
          <Card>
            <CardHeader title="API Configuration" subheader="Configure API server settings" />
            <CardContent>
              <Grid container spacing={3}>
                <Grid item xs={12} md={6}>
                  <TextField
                    label="Host"
                    value={config.api.host}
                    onChange={(e) => handleConfigChange("api", "host", e.target.value)}
                    fullWidth
                  />
                </Grid>
                <Grid item xs={12} md={6}>
                  <TextField
                    label="Port"
                    type="number"
                    value={config.api.port}
                    onChange={(e) => handleConfigChange("api", "port", parseInt(e.target.value))}
                    fullWidth
                  />
                </Grid>
                <Grid item xs={12} md={6}>
                  <FormControlLabel
                    control={
                      <Switch
                        checked={config.api.auth_enabled}
                        onChange={(e) =>
                          handleConfigChange("api", "auth_enabled", e.target.checked)
                        }
                      />
                    }
                    label="Authentication Enabled"
                  />
                </Grid>
                <Grid item xs={12} md={6}>
                  <FormControlLabel
                    control={
                      <Switch
                        checked={config.api.cors_enabled}
                        onChange={(e) =>
                          handleConfigChange("api", "cors_enabled", e.target.checked)
                        }
                      />
                    }
                    label="CORS Enabled"
                  />
                </Grid>
                <Grid item xs={12} md={6}>
                  <FormControlLabel
                    control={
                      <Switch
                        checked={config.api.rate_limit_enabled}
                        onChange={(e) =>
                          handleConfigChange("api", "rate_limit_enabled", e.target.checked)
                        }
                      />
                    }
                    label="Rate Limiting Enabled"
                  />
                </Grid>
                {config.api.rate_limit_enabled && (
                  <>
                    <Grid item xs={12} md={6}>
                      <TextField
                        label="Rate Limit Requests"
                        type="number"
                        value={config.api.rate_limit_requests}
                        onChange={(e) =>
                          handleConfigChange("api", "rate_limit_requests", parseInt(e.target.value))
                        }
                        fullWidth
                      />
                    </Grid>
                    <Grid item xs={12} md={6}>
                      <TextField
                        label="Rate Limit Window"
                        value={config.api.rate_limit_window}
                        onChange={(e) =>
                          handleConfigChange("api", "rate_limit_window", e.target.value)
                        }
                        fullWidth
                        helperText="Duration format (e.g., '1m', '60s')"
                      />
                    </Grid>
                  </>
                )}
              </Grid>
            </CardContent>
          </Card>
        </TabPanel>

        {/* Database Configuration Tab */}
        <TabPanel value={activeTab} index={1}>
          <Card>
            <CardHeader
              title="Database Configuration"
              subheader="Configure database connection settings"
            />
            <CardContent>
              <Grid container spacing={3}>
                <Grid item xs={12} md={6}>
                  <TextField
                    label="Host"
                    value={config.database.host}
                    onChange={(e) => handleConfigChange("database", "host", e.target.value)}
                    fullWidth
                  />
                </Grid>
                <Grid item xs={12} md={6}>
                  <TextField
                    label="Port"
                    type="number"
                    value={config.database.port}
                    onChange={(e) =>
                      handleConfigChange("database", "port", parseInt(e.target.value))
                    }
                    fullWidth
                  />
                </Grid>
                <Grid item xs={12} md={6}>
                  <TextField
                    label="Database Name"
                    value={config.database.database}
                    onChange={(e) => handleConfigChange("database", "database", e.target.value)}
                    fullWidth
                  />
                </Grid>
                <Grid item xs={12} md={6}>
                  <TextField
                    label="Username"
                    value={config.database.username}
                    onChange={(e) => handleConfigChange("database", "username", e.target.value)}
                    fullWidth
                  />
                </Grid>
                <Grid item xs={12} md={6}>
                  <FormControl fullWidth>
                    <InputLabel>SSL Mode</InputLabel>
                    <Select
                      value={config.database.ssl_mode}
                      onChange={(e) => handleConfigChange("database", "ssl_mode", e.target.value)}
                      label="SSL Mode"
                    >
                      <MenuItem value="disable">Disable</MenuItem>
                      <MenuItem value="require">Require</MenuItem>
                      <MenuItem value="verify-ca">Verify CA</MenuItem>
                      <MenuItem value="verify-full">Verify Full</MenuItem>
                    </Select>
                  </FormControl>
                </Grid>
                <Grid item xs={12} md={6}>
                  <TextField
                    label="Max Connections"
                    type="number"
                    value={config.database.max_connections}
                    onChange={(e) =>
                      handleConfigChange("database", "max_connections", parseInt(e.target.value))
                    }
                    fullWidth
                  />
                </Grid>
                <Grid item xs={12} md={6}>
                  <TextField
                    label="Connection Timeout"
                    value={config.database.connection_timeout}
                    onChange={(e) =>
                      handleConfigChange("database", "connection_timeout", e.target.value)
                    }
                    fullWidth
                    helperText="Duration format (e.g., '30s', '1m')"
                  />
                </Grid>
              </Grid>
            </CardContent>
          </Card>
        </TabPanel>

        {/* Logging Configuration Tab */}
        <TabPanel value={activeTab} index={2}>
          <Card>
            <CardHeader
              title="Logging Configuration"
              subheader="Configure logging level and output settings"
            />
            <CardContent>
              <Grid container spacing={3}>
                <Grid item xs={12} md={6}>
                  <FormControl fullWidth>
                    <InputLabel>Log Level</InputLabel>
                    <Select
                      value={config.logging.level}
                      onChange={(e) => handleConfigChange("logging", "level", e.target.value)}
                      label="Log Level"
                    >
                      <MenuItem value="debug">Debug</MenuItem>
                      <MenuItem value="info">Info</MenuItem>
                      <MenuItem value="warn">Warning</MenuItem>
                      <MenuItem value="error">Error</MenuItem>
                    </Select>
                  </FormControl>
                </Grid>
                <Grid item xs={12} md={6}>
                  <FormControl fullWidth>
                    <InputLabel>Log Format</InputLabel>
                    <Select
                      value={config.logging.format}
                      onChange={(e) => handleConfigChange("logging", "format", e.target.value)}
                      label="Log Format"
                    >
                      <MenuItem value="text">Text</MenuItem>
                      <MenuItem value="json">JSON</MenuItem>
                    </Select>
                  </FormControl>
                </Grid>
                <Grid item xs={12}>
                  <TextField
                    label="Log Output"
                    value={config.logging.output}
                    onChange={(e) => handleConfigChange("logging", "output", e.target.value)}
                    fullWidth
                    helperText="File path for log output (leave empty for stdout)"
                  />
                </Grid>
              </Grid>
            </CardContent>
          </Card>

          <Card sx={{ mt: 3 }}>
            <CardHeader title="Scan Defaults" subheader="Configure default scan parameters" />
            <CardContent>
              <Grid container spacing={3}>
                <Grid item xs={12} md={4}>
                  <TextField
                    label="Default Timeout"
                    value={config.scan_defaults.timeout}
                    onChange={(e) => handleConfigChange("scan_defaults", "timeout", e.target.value)}
                    fullWidth
                    helperText="Duration format (e.g., '30s', '5m')"
                  />
                </Grid>
                <Grid item xs={12} md={4}>
                  <TextField
                    label="Max Concurrent Scans"
                    type="number"
                    value={config.scan_defaults.max_concurrent}
                    onChange={(e) =>
                      handleConfigChange(
                        "scan_defaults",
                        "max_concurrent",
                        parseInt(e.target.value),
                      )
                    }
                    fullWidth
                  />
                </Grid>
                <Grid item xs={12} md={4}>
                  <TextField
                    label="Retry Attempts"
                    type="number"
                    value={config.scan_defaults.retry_attempts}
                    onChange={(e) =>
                      handleConfigChange(
                        "scan_defaults",
                        "retry_attempts",
                        parseInt(e.target.value),
                      )
                    }
                    fullWidth
                  />
                </Grid>
              </Grid>
            </CardContent>
          </Card>
        </TabPanel>

        {/* Security Tab */}
        <TabPanel value={activeTab} index={3}>
          <Card>
            <CardHeader
              title="Security Settings"
              subheader="Manage authentication and security options"
            />
            <CardContent>
              <Stack spacing={3}>
                <Alert severity="info">
                  Security settings require system restart to take effect.
                </Alert>

                <Accordion>
                  <AccordionSummary expandIcon={<ExpandMoreIcon />}>
                    <Typography variant="h6">Authentication</Typography>
                  </AccordionSummary>
                  <AccordionDetails>
                    <Stack spacing={2}>
                      <FormControlLabel
                        control={
                          <Switch
                            checked={config.api.auth_enabled}
                            onChange={(e) =>
                              handleConfigChange("api", "auth_enabled", e.target.checked)
                            }
                          />
                        }
                        label="Enable Authentication"
                      />
                      <Typography variant="body2" color="text.secondary">
                        When enabled, all API endpoints will require valid authentication tokens.
                      </Typography>
                    </Stack>
                  </AccordionDetails>
                </Accordion>

                <Accordion>
                  <AccordionSummary expandIcon={<ExpandMoreIcon />}>
                    <Typography variant="h6">CORS Settings</Typography>
                  </AccordionSummary>
                  <AccordionDetails>
                    <Stack spacing={2}>
                      <FormControlLabel
                        control={
                          <Switch
                            checked={config.api.cors_enabled}
                            onChange={(e) =>
                              handleConfigChange("api", "cors_enabled", e.target.checked)
                            }
                          />
                        }
                        label="Enable CORS"
                      />
                      <Typography variant="body2" color="text.secondary">
                        Cross-Origin Resource Sharing allows web applications from different domains
                        to access the API.
                      </Typography>
                    </Stack>
                  </AccordionDetails>
                </Accordion>

                <Accordion>
                  <AccordionSummary expandIcon={<ExpandMoreIcon />}>
                    <Typography variant="h6">Database Security</Typography>
                  </AccordionSummary>
                  <AccordionDetails>
                    <Stack spacing={2}>
                      <FormControl fullWidth>
                        <InputLabel>SSL Mode</InputLabel>
                        <Select
                          value={config.database.ssl_mode}
                          onChange={(e) =>
                            handleConfigChange("database", "ssl_mode", e.target.value)
                          }
                          label="SSL Mode"
                        >
                          <MenuItem value="disable">Disable</MenuItem>
                          <MenuItem value="require">Require</MenuItem>
                          <MenuItem value="verify-ca">Verify CA</MenuItem>
                          <MenuItem value="verify-full">Verify Full</MenuItem>
                        </Select>
                      </FormControl>
                      <Typography variant="body2" color="text.secondary">
                        SSL configuration for database connections. Higher levels provide better
                        security.
                      </Typography>
                    </Stack>
                  </AccordionDetails>
                </Accordion>
              </Stack>
            </CardContent>
          </Card>
        </TabPanel>

        {/* System Status Tab */}
        <TabPanel value={activeTab} index={4}>
          {/* System Status */}
          {systemStatus && (
            <Card sx={{ mb: 3 }}>
              <CardHeader
                title="System Status"
                subheader="Current system health and performance metrics"
                action={
                  <Chip
                    label={systemStatus.status}
                    color={getHealthStatusColor(systemStatus.status)}
                    icon={getHealthStatusIcon(systemStatus.status)}
                  />
                }
              />
              <CardContent>
                <Grid container spacing={3}>
                  <Grid item xs={12} md={6}>
                    <List>
                      <ListItem>
                        <ListItemText primary="Version" secondary={systemStatus.version} />
                      </ListItem>
                      <ListItem>
                        <ListItemText primary="Uptime" secondary={systemStatus.uptime} />
                      </ListItem>
                      <ListItem>
                        <ListItemText
                          primary="Active Scans"
                          secondary={systemStatus.active_scans}
                        />
                      </ListItem>
                      <ListItem>
                        <ListItemText
                          primary="Active Discoveries"
                          secondary={systemStatus.active_discoveries}
                        />
                      </ListItem>
                    </List>
                  </Grid>
                  <Grid item xs={12} md={6}>
                    <List>
                      <ListItem>
                        <ListItemText
                          primary="Database Status"
                          secondary={
                            <Chip
                              label={systemStatus.database.status}
                              color={getHealthStatusColor(systemStatus.database.status)}
                              size="small"
                            />
                          }
                        />
                      </ListItem>
                      <ListItem>
                        <ListItemText
                          primary="Database Connections"
                          secondary={systemStatus.database.connections}
                        />
                      </ListItem>
                      <ListItem>
                        <ListItemText
                          primary="Memory Usage"
                          secondary={`${systemStatus.memory.percentage}% (${(systemStatus.memory.used / 1024 / 1024).toFixed(0)}MB / ${(systemStatus.memory.total / 1024 / 1024).toFixed(0)}MB)`}
                        />
                      </ListItem>
                      <ListItem>
                        <ListItemText
                          primary="WebSocket Connections"
                          secondary={systemStatus.websocket_connections}
                        />
                      </ListItem>
                    </List>
                  </Grid>
                </Grid>
              </CardContent>
            </Card>
          )}

          {/* Health Checks */}
          {healthCheck && (
            <Card>
              <CardHeader
                title="Health Checks"
                subheader="Detailed system component health status"
                action={
                  <Chip
                    label={healthCheck.status}
                    color={getHealthStatusColor(healthCheck.status)}
                    icon={getHealthStatusIcon(healthCheck.status)}
                  />
                }
              />
              <CardContent>
                <Stack spacing={2}>
                  {Object.entries(healthCheck.checks).map(([component, check]) => (
                    <Paper key={component} variant="outlined" sx={{ p: 2 }}>
                      <Box display="flex" alignItems="center" justifyContent="space-between">
                        <Box display="flex" alignItems="center" gap={1}>
                          {getHealthStatusIcon(check.status)}
                          <Typography variant="h6" sx={{ textTransform: "capitalize" }}>
                            {component}
                          </Typography>
                        </Box>
                        <Chip
                          label={check.status}
                          color={getHealthStatusColor(check.status)}
                          size="small"
                        />
                      </Box>
                      {check.message && (
                        <Typography variant="body2" color="text.secondary" sx={{ mt: 1 }}>
                          {check.message}
                        </Typography>
                      )}
                      {"latency" in check && check.latency && (
                        <Typography variant="body2" color="text.secondary">
                          Latency: {check.latency}ms
                        </Typography>
                      )}
                    </Paper>
                  ))}
                </Stack>
              </CardContent>
            </Card>
          )}
        </TabPanel>

        {/* Error Logs Tab */}
        <TabPanel value={activeTab} index={5}>
          <Card>
            <CardHeader
              title="Error Logs"
              subheader="View and export application error logs for debugging"
              action={
                <Box sx={{ display: "flex", gap: 1 }}>
                  <FormControl size="small" sx={{ minWidth: 120 }}>
                    <InputLabel>Filter</InputLabel>
                    <Select
                      value={logFilter}
                      onChange={(e) => setLogFilter(e.target.value)}
                      label="Filter"
                    >
                      <MenuItem value="all">All Logs</MenuItem>
                      <MenuItem value="error">Errors Only</MenuItem>
                      <MenuItem value="warn">Warnings Only</MenuItem>
                      <MenuItem value="info">Info Only</MenuItem>
                    </Select>
                  </FormControl>
                  <Button startIcon={<RefreshIcon />} onClick={loadErrorLogs} size="small">
                    Refresh
                  </Button>
                  <Button
                    startIcon={<SaveIcon />}
                    onClick={() => {
                      const dataStr = JSON.stringify(errorLogs, null, 2);
                      const dataBlob = new Blob([dataStr], {
                        type: "application/json",
                      });
                      const url = URL.createObjectURL(dataBlob);
                      const link = document.createElement("a");
                      link.href = url;
                      link.download = `scanorama-logs-${new Date().toISOString()}.json`;
                      link.click();
                      URL.revokeObjectURL(url);
                    }}
                    size="small"
                  >
                    Export
                  </Button>
                </Box>
              }
            />
            <CardContent>
              {errorLogs.length === 0 ? (
                <Box sx={{ textAlign: "center", py: 4 }}>
                  <InfoIcon sx={{ fontSize: 48, color: "text.secondary", mb: 2 }} />
                  <Typography variant="h6" color="text.secondary">
                    No error logs found
                  </Typography>
                  <Typography variant="body2" color="text.secondary">
                    This is good news! No errors have been logged recently.
                  </Typography>
                </Box>
              ) : (
                <Box>
                  <Typography variant="body2" sx={{ mb: 2 }}>
                    Showing{" "}
                    {
                      errorLogs.filter(
                        (log) =>
                          logFilter === "all" ||
                          log.level.toLowerCase().includes(logFilter.toLowerCase()),
                      ).length
                    }{" "}
                    of {errorLogs.length} log entries
                  </Typography>

                  <Box sx={{ maxHeight: 400, overflow: "auto" }}>
                    {errorLogs
                      .filter(
                        (log) =>
                          logFilter === "all" ||
                          log.level.toLowerCase().includes(logFilter.toLowerCase()),
                      )
                      .slice(0, 100) // Limit to prevent performance issues
                      .map((log) => (
                        <Accordion key={log.id} sx={{ mb: 1 }}>
                          <AccordionSummary expandIcon={<ExpandMoreIcon />}>
                            <Box
                              sx={{
                                display: "flex",
                                alignItems: "center",
                                gap: 2,
                                width: "100%",
                              }}
                            >
                              <Chip
                                label={log.level}
                                size="small"
                                color={
                                  log.level === "ERROR"
                                    ? "error"
                                    : log.level === "WARN"
                                      ? "warning"
                                      : log.level === "INFO"
                                        ? "info"
                                        : "default"
                                }
                              />
                              <Typography variant="body2" sx={{ flexGrow: 1 }}>
                                {log.message}
                              </Typography>
                              <Typography variant="caption" color="text.secondary">
                                {new Date(log.timestamp).toLocaleString()}
                              </Typography>
                            </Box>
                          </AccordionSummary>
                          <AccordionDetails>
                            <Box sx={{ bgcolor: "grey.50", p: 2, borderRadius: 1 }}>
                              {log.component && (
                                <Typography variant="body2" sx={{ mb: 1 }}>
                                  <strong>Component:</strong> {log.component}
                                </Typography>
                              )}
                              {log.error && (
                                <>
                                  <Typography variant="body2" sx={{ mb: 1 }}>
                                    <strong>Error:</strong> {log.error.message}
                                  </Typography>
                                  {log.error.stack && (
                                    <Typography
                                      variant="caption"
                                      component="pre"
                                      sx={{
                                        display: "block",
                                        bgcolor: "grey.100",
                                        p: 1,
                                        borderRadius: 1,
                                        overflow: "auto",
                                        maxHeight: 200,
                                        fontFamily: "monospace",
                                        fontSize: "0.75rem",
                                      }}
                                    >
                                      {log.error.stack}
                                    </Typography>
                                  )}
                                </>
                              )}
                              {log.metadata && Object.keys(log.metadata).length > 0 && (
                                <>
                                  <Typography variant="body2" sx={{ mb: 1, mt: 2 }}>
                                    <strong>Metadata:</strong>
                                  </Typography>
                                  <Typography
                                    variant="caption"
                                    component="pre"
                                    sx={{
                                      display: "block",
                                      bgcolor: "grey.100",
                                      p: 1,
                                      borderRadius: 1,
                                      overflow: "auto",
                                      maxHeight: 200,
                                      fontFamily: "monospace",
                                      fontSize: "0.75rem",
                                    }}
                                  >
                                    {JSON.stringify(log.metadata, null, 2)}
                                  </Typography>
                                </>
                              )}
                            </Box>
                          </AccordionDetails>
                        </Accordion>
                      ))}
                  </Box>

                  <Box sx={{ mt: 2, display: "flex", gap: 2 }}>
                    <Button
                      variant="outlined"
                      color="error"
                      onClick={() => {
                        logger.clearLogs();
                        setErrorLogs([]);
                      }}
                    >
                      Clear All Logs
                    </Button>
                    <Typography
                      variant="caption"
                      color="text.secondary"
                      sx={{ alignSelf: "center" }}
                    >
                      Session ID: {logger.getSessionId()}
                    </Typography>
                  </Box>
                </Box>
              )}
            </CardContent>
          </Card>
        </TabPanel>
      </Paper>

      {/* Save Confirmation Dialog */}
      <Dialog open={confirmDialog} onClose={() => setConfirmDialog(false)}>
        <DialogTitle>Confirm Settings Changes</DialogTitle>
        <DialogContent>
          <Alert severity="warning" sx={{ mb: 2 }}>
            Some configuration changes may require a system restart to take effect.
          </Alert>
          <Typography>Are you sure you want to save these configuration changes?</Typography>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setConfirmDialog(false)}>Cancel</Button>
          <Button onClick={confirmSaveSettings} variant="contained" disabled={saving}>
            {saving ? "Saving..." : "Save Changes"}
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
};

export default Settings;
