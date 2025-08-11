import React, { useState, useEffect } from "react";
import { useParams, useNavigate } from "react-router-dom";
import {
  Box,
  Card,
  CardContent,
  Typography,
  Button,
  Grid,
  Chip,
  LinearProgress,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  TablePagination,
  Alert,
  Skeleton,
  Stack,
  Paper,
  Tabs,
  Tab,
  List,
  ListItem,
  ListItemText,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Tooltip,
} from "@mui/material";
import {
  ArrowBack as BackIcon,
  PlayArrow as StartIcon,
  Stop as StopIcon,
  Refresh as RefreshIcon,
  Download as DownloadIcon,
  Edit as EditIcon,
  Delete as DeleteIcon,
  Computer as HostIcon,
  Security as SecurityIcon,
  Speed as SpeedIcon,
  Schedule as ScheduleIcon,
} from "@mui/icons-material";
import { format, formatDistanceToNow } from "date-fns";

import { useScan, useScanResults, useScanMutations, usePagination } from "../../hooks/useApi";
import { ScanResult, PortState } from "../../types/api";

interface TabPanelProps {
  children?: React.ReactNode;
  index: number;
  value: number;
}

const TabPanel: React.FC<TabPanelProps> = ({ children, value, index, ...other }) => (
  <div
    role="tabpanel"
    hidden={value !== index}
    id={`scan-tabpanel-${index}`}
    aria-labelledby={`scan-tab-${index}`}
    {...other}
  >
    {value === index && <Box sx={{ py: 3 }}>{children}</Box>}
  </div>
);

const ScanDetail: React.FC = () => {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();

  // State
  const [currentTab, setCurrentTab] = useState(0);
  const [deleteDialog, setDeleteDialog] = useState(false);

  // API hooks
  const {
    data: scan,
    loading: scanLoading,
    error: scanError,
    refetch: refetchScan,
  } = useScan(id || null);
  const pagination = usePagination(1, 25);
  const {
    data: results,
    loading: resultsLoading,
    error: resultsError,
    refetch: refetchResults,
  } = useScanResults(id || null, pagination.params);
  const mutations = useScanMutations();

  // Auto-refresh for running scans
  useEffect(() => {
    if (scan?.status === "running") {
      const interval = setInterval(() => {
        refetchScan();
        refetchResults();
      }, 5000);
      return () => clearInterval(interval);
    }
    return undefined;
  }, [scan?.status, refetchScan, refetchResults]);

  // Event handlers
  const handleStartScan = async () => {
    if (id) {
      await mutations.start.execute(id);
      refetchScan();
    }
  };

  const handleStopScan = async () => {
    if (id) {
      await mutations.stop.execute(id);
      refetchScan();
    }
  };

  const handleDeleteScan = async () => {
    if (id) {
      const success = await mutations.delete.execute(id);
      if (success) {
        navigate("/scans");
      }
    }
  };

  const handleExportResults = () => {
    if (results?.results) {
      const csv = convertResultsToCSV(results.results);
      downloadCSV(csv, `scan-${scan?.name || id}-results.csv`);
    }
  };

  // Utility functions
  const convertResultsToCSV = (scanResults: ScanResult[]): string => {
    const headers = [
      "Host IP",
      "Hostname",
      "Port",
      "Protocol",
      "State",
      "Service",
      "Version",
      "Banner",
    ];
    const rows = scanResults.map((result) => [
      result.host_ip,
      result.hostname || "",
      result.port.toString(),
      result.protocol,
      result.state,
      result.service || "",
      result.version || "",
      result.banner || "",
    ]);

    return [headers, ...rows]
      .map((row) => row.map((cell) => `"${cell.replace(/"/g, '""')}"`).join(","))
      .join("\n");
  };

  const downloadCSV = (content: string, filename: string) => {
    const blob = new Blob([content], { type: "text/csv;charset=utf-8;" });
    const link = document.createElement("a");
    const url = URL.createObjectURL(blob);
    link.setAttribute("href", url);
    link.setAttribute("download", filename);
    link.style.visibility = "hidden";
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
  };

  const getPortStateColor = (state: PortState) => {
    switch (state) {
      case "open":
        return "success";
      case "closed":
        return "default";
      case "filtered":
        return "warning";
      default:
        return "info";
    }
  };

  // Loading state
  if (scanLoading && !scan) {
    return (
      <Box>
        <Skeleton variant="rectangular" width="100%" height={200} sx={{ mb: 3 }} />
        <Grid container spacing={3}>
          {[1, 2, 3, 4].map((i) => (
            <Grid item xs={12} sm={6} md={3} key={i}>
              <Skeleton variant="rectangular" height={120} />
            </Grid>
          ))}
        </Grid>
      </Box>
    );
  }

  // Error state
  if (scanError || !scan) {
    return (
      <Box>
        <Button startIcon={<BackIcon />} onClick={() => navigate("/scans")} sx={{ mb: 2 }}>
          Back to Scans
        </Button>
        <Alert severity="error">{scanError?.message || "Scan not found"}</Alert>
      </Box>
    );
  }

  return (
    <Box>
      {/* Header */}
      <Box sx={{ mb: 4 }}>
        <Button startIcon={<BackIcon />} onClick={() => navigate("/scans")} sx={{ mb: 2 }}>
          Back to Scans
        </Button>

        <Box
          sx={{
            display: "flex",
            justifyContent: "space-between",
            alignItems: "center",
            mb: 2,
          }}
        >
          <Box>
            <Typography variant="h4" component="h1" sx={{ fontWeight: "bold", mb: 1 }}>
              {scan.name}
            </Typography>
            <Typography variant="body1" color="text.secondary">
              {scan.description || "No description provided"}
            </Typography>
          </Box>

          <Stack direction="row" spacing={2}>
            {scan.status === "running" ? (
              <Button
                variant="contained"
                color="error"
                startIcon={<StopIcon />}
                onClick={handleStopScan}
                disabled={mutations.stop.loading}
              >
                Stop Scan
              </Button>
            ) : (
              <Button
                variant="contained"
                startIcon={<StartIcon />}
                onClick={handleStartScan}
                disabled={mutations.start.loading}
              >
                Start Scan
              </Button>
            )}

            <Button
              variant="outlined"
              startIcon={<EditIcon />}
              onClick={() => navigate(`/scans/${id}/edit`)}
            >
              Edit
            </Button>

            <Button
              variant="outlined"
              color="error"
              startIcon={<DeleteIcon />}
              onClick={() => setDeleteDialog(true)}
            >
              Delete
            </Button>
          </Stack>
        </Box>
      </Box>

      {/* Status and progress */}
      <Grid container spacing={3} sx={{ mb: 4 }}>
        <Grid item xs={12} sm={6} md={3}>
          <Card>
            <CardContent sx={{ textAlign: "center" }}>
              <SecurityIcon sx={{ fontSize: 40, color: "primary.main", mb: 1 }} />
              <Typography variant="h6" sx={{ fontWeight: "bold" }}>
                Status
              </Typography>
              <Chip
                label={scan.status.toUpperCase()}
                color={
                  scan.status === "completed"
                    ? "success"
                    : scan.status === "running"
                      ? "primary"
                      : scan.status === "failed"
                        ? "error"
                        : "default"
                }
                sx={{ mt: 1 }}
              />
            </CardContent>
          </Card>
        </Grid>

        <Grid item xs={12} sm={6} md={3}>
          <Card>
            <CardContent sx={{ textAlign: "center" }}>
              <SpeedIcon sx={{ fontSize: 40, color: "secondary.main", mb: 1 }} />
              <Typography variant="h6" sx={{ fontWeight: "bold" }}>
                Progress
              </Typography>
              <Typography variant="h4" sx={{ mt: 1 }}>
                {Math.round(scan.progress)}%
              </Typography>
              {scan.status === "running" && (
                <LinearProgress variant="determinate" value={scan.progress} sx={{ mt: 1 }} />
              )}
            </CardContent>
          </Card>
        </Grid>

        <Grid item xs={12} sm={6} md={3}>
          <Card>
            <CardContent sx={{ textAlign: "center" }}>
              <HostIcon sx={{ fontSize: 40, color: "info.main", mb: 1 }} />
              <Typography variant="h6" sx={{ fontWeight: "bold" }}>
                Targets
              </Typography>
              <Typography variant="h4" sx={{ mt: 1 }}>
                {scan.targets.length}
              </Typography>
            </CardContent>
          </Card>
        </Grid>

        <Grid item xs={12} sm={6} md={3}>
          <Card>
            <CardContent sx={{ textAlign: "center" }}>
              <ScheduleIcon sx={{ fontSize: 40, color: "warning.main", mb: 1 }} />
              <Typography variant="h6" sx={{ fontWeight: "bold" }}>
                Duration
              </Typography>
              <Typography variant="h4" sx={{ mt: 1 }}>
                {scan.duration || "0s"}
              </Typography>
            </CardContent>
          </Card>
        </Grid>
      </Grid>

      {/* Tabs */}
      <Card>
        <Box sx={{ borderBottom: 1, borderColor: "divider" }}>
          <Tabs value={currentTab} onChange={(_, newValue) => setCurrentTab(newValue)}>
            <Tab label="Results" />
            <Tab label="Configuration" />
            <Tab label="Timeline" />
          </Tabs>
        </Box>

        {/* Results tab */}
        <TabPanel value={currentTab} index={0}>
          {resultsError && (
            <Alert severity="error" sx={{ mb: 2 }}>
              Failed to load scan results: {resultsError.message}
            </Alert>
          )}

          <Box
            sx={{
              display: "flex",
              justifyContent: "space-between",
              alignItems: "center",
              mb: 2,
            }}
          >
            <Typography variant="h6">
              Scan Results ({results?.total_ports || 0} ports scanned)
            </Typography>
            <Stack direction="row" spacing={2}>
              <Button
                startIcon={<RefreshIcon />}
                onClick={refetchResults}
                disabled={resultsLoading}
                size="small"
              >
                Refresh
              </Button>
              {results?.results && results.results.length > 0 && (
                <Button
                  startIcon={<DownloadIcon />}
                  onClick={handleExportResults}
                  variant="outlined"
                  size="small"
                >
                  Export CSV
                </Button>
              )}
            </Stack>
          </Box>

          {/* Results summary */}
          {results && (
            <Grid container spacing={2} sx={{ mb: 3 }}>
              <Grid item xs={6} sm={3}>
                <Paper sx={{ p: 2, textAlign: "center" }}>
                  <Typography variant="h5" color="success.main" sx={{ fontWeight: "bold" }}>
                    {results.open_ports}
                  </Typography>
                  <Typography variant="body2" color="text.secondary">
                    Open Ports
                  </Typography>
                </Paper>
              </Grid>
              <Grid item xs={6} sm={3}>
                <Paper sx={{ p: 2, textAlign: "center" }}>
                  <Typography variant="h5" color="text.secondary" sx={{ fontWeight: "bold" }}>
                    {results.closed_ports}
                  </Typography>
                  <Typography variant="body2" color="text.secondary">
                    Closed Ports
                  </Typography>
                </Paper>
              </Grid>
              <Grid item xs={6} sm={3}>
                <Paper sx={{ p: 2, textAlign: "center" }}>
                  <Typography variant="h5" color="primary.main" sx={{ fontWeight: "bold" }}>
                    {results.total_hosts}
                  </Typography>
                  <Typography variant="body2" color="text.secondary">
                    Hosts
                  </Typography>
                </Paper>
              </Grid>
              <Grid item xs={6} sm={3}>
                <Paper sx={{ p: 2, textAlign: "center" }}>
                  <Typography variant="h5" color="info.main" sx={{ fontWeight: "bold" }}>
                    {results.total_ports}
                  </Typography>
                  <Typography variant="body2" color="text.secondary">
                    Total Ports
                  </Typography>
                </Paper>
              </Grid>
            </Grid>
          )}

          {/* Results table */}
          <TableContainer>
            <Table>
              <TableHead>
                <TableRow>
                  <TableCell>Host</TableCell>
                  <TableCell>Port</TableCell>
                  <TableCell>Protocol</TableCell>
                  <TableCell>State</TableCell>
                  <TableCell>Service</TableCell>
                  <TableCell>Version</TableCell>
                  <TableCell>Scan Time</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {resultsLoading ? (
                  Array.from({ length: 10 }).map((_, index) => (
                    <TableRow key={index}>
                      {Array.from({ length: 7 }).map((_, cellIndex) => (
                        <TableCell key={cellIndex}>
                          <Skeleton variant="text" />
                        </TableCell>
                      ))}
                    </TableRow>
                  ))
                ) : results?.results?.length ? (
                  results.results.map((result) => (
                    <TableRow key={result.id} hover>
                      <TableCell>
                        <Box>
                          <Typography variant="body2" sx={{ fontWeight: 500 }}>
                            {result.host_ip}
                          </Typography>
                          {result.hostname && (
                            <Typography variant="caption" color="text.secondary">
                              {result.hostname}
                            </Typography>
                          )}
                        </Box>
                      </TableCell>
                      <TableCell>
                        <Typography variant="body2" sx={{ fontFamily: "monospace" }}>
                          {result.port}
                        </Typography>
                      </TableCell>
                      <TableCell>
                        <Chip
                          label={result.protocol.toUpperCase()}
                          size="small"
                          variant="outlined"
                        />
                      </TableCell>
                      <TableCell>
                        <Chip
                          label={result.state}
                          size="small"
                          color={getPortStateColor(result.state)}
                        />
                      </TableCell>
                      <TableCell>
                        {result.service ? (
                          <Typography variant="body2">{result.service}</Typography>
                        ) : (
                          <Typography variant="body2" color="text.secondary">
                            Unknown
                          </Typography>
                        )}
                      </TableCell>
                      <TableCell>
                        {result.version ? (
                          <Typography
                            variant="body2"
                            sx={{ fontFamily: "monospace", fontSize: "0.8rem" }}
                          >
                            {result.version}
                          </Typography>
                        ) : (
                          <Typography variant="body2" color="text.secondary">
                            -
                          </Typography>
                        )}
                      </TableCell>
                      <TableCell>
                        <Tooltip title={format(new Date(result.scan_time), "PPpp")}>
                          <Typography variant="body2">
                            {formatDistanceToNow(new Date(result.scan_time), {
                              addSuffix: true,
                            })}
                          </Typography>
                        </Tooltip>
                      </TableCell>
                    </TableRow>
                  ))
                ) : (
                  <TableRow>
                    <TableCell colSpan={7} align="center" sx={{ py: 6 }}>
                      <Typography variant="body1" color="text.secondary">
                        {scan.status === "pending" ? "Scan not started yet" : "No results found"}
                      </Typography>
                    </TableCell>
                  </TableRow>
                )}
              </TableBody>
            </Table>
          </TableContainer>

          {/* Pagination */}
          {results && results.total_ports > pagination.pageSize && (
            <TablePagination
              rowsPerPageOptions={[10, 25, 50, 100]}
              component="div"
              count={results.total_ports}
              rowsPerPage={pagination.pageSize}
              page={pagination.page - 1}
              onPageChange={(_, newPage) => pagination.goToPage(newPage + 1)}
              onRowsPerPageChange={(e) => pagination.changePageSize(parseInt(e.target.value))}
            />
          )}
        </TabPanel>

        {/* Configuration tab */}
        <TabPanel value={currentTab} index={1}>
          <Grid container spacing={3}>
            <Grid item xs={12} md={6}>
              <Typography variant="h6" sx={{ mb: 2, fontWeight: "bold" }}>
                Scan Configuration
              </Typography>
              <List>
                <ListItem>
                  <ListItemText primary="Scan Type" secondary={scan.scan_type.toUpperCase()} />
                </ListItem>
                <ListItem>
                  <ListItemText primary="Targets" secondary={scan.targets.join(", ")} />
                </ListItem>
                <ListItem>
                  <ListItemText primary="Ports" secondary={scan.ports || "Default ports"} />
                </ListItem>
                <ListItem>
                  <ListItemText primary="Profile ID" secondary={scan.profile_id || "None"} />
                </ListItem>
                <ListItem>
                  <ListItemText primary="Schedule ID" secondary={scan.schedule_id || "Manual"} />
                </ListItem>
              </List>
            </Grid>

            <Grid item xs={12} md={6}>
              <Typography variant="h6" sx={{ mb: 2, fontWeight: "bold" }}>
                Timing Information
              </Typography>
              <List>
                <ListItem>
                  <ListItemText
                    primary="Created"
                    secondary={format(new Date(scan.created_at), "PPpp")}
                  />
                </ListItem>
                <ListItem>
                  <ListItemText
                    primary="Last Updated"
                    secondary={format(new Date(scan.updated_at), "PPpp")}
                  />
                </ListItem>
                {scan.start_time && (
                  <ListItem>
                    <ListItemText
                      primary="Started"
                      secondary={format(new Date(scan.start_time), "PPpp")}
                    />
                  </ListItem>
                )}
                {scan.end_time && (
                  <ListItem>
                    <ListItemText
                      primary="Completed"
                      secondary={format(new Date(scan.end_time), "PPpp")}
                    />
                  </ListItem>
                )}
                <ListItem>
                  <ListItemText primary="Duration" secondary={scan.duration || "Not completed"} />
                </ListItem>
                {scan.created_by && (
                  <ListItem>
                    <ListItemText primary="Created By" secondary={scan.created_by} />
                  </ListItem>
                )}
              </List>
            </Grid>

            {/* Options and tags */}
            {scan.options && Object.keys(scan.options).length > 0 && (
              <Grid item xs={12}>
                <Typography variant="h6" sx={{ mb: 2, fontWeight: "bold" }}>
                  Scan Options
                </Typography>
                <Paper sx={{ p: 2 }}>
                  {Object.entries(scan.options).map(([key, value]) => (
                    <Box key={key} sx={{ mb: 1 }}>
                      <Typography variant="body2" component="span" sx={{ fontWeight: 500 }}>
                        {key}:
                      </Typography>
                      <Typography
                        variant="body2"
                        component="span"
                        sx={{ ml: 1, fontFamily: "monospace" }}
                      >
                        {value}
                      </Typography>
                    </Box>
                  ))}
                </Paper>
              </Grid>
            )}

            {scan.tags && scan.tags.length > 0 && (
              <Grid item xs={12}>
                <Typography variant="h6" sx={{ mb: 2, fontWeight: "bold" }}>
                  Tags
                </Typography>
                <Box>
                  {scan.tags.map((tag) => (
                    <Chip key={tag} label={tag} variant="outlined" sx={{ mr: 1, mb: 1 }} />
                  ))}
                </Box>
              </Grid>
            )}
          </Grid>
        </TabPanel>

        {/* Timeline tab */}
        <TabPanel value={currentTab} index={2}>
          <Typography variant="h6" sx={{ mb: 2, fontWeight: "bold" }}>
            Scan Timeline
          </Typography>
          <List>
            <ListItem>
              <ListItemText
                primary="Scan Created"
                secondary={format(new Date(scan.created_at), "PPpp")}
              />
            </ListItem>
            {scan.start_time && (
              <ListItem>
                <ListItemText
                  primary="Scan Started"
                  secondary={format(new Date(scan.start_time), "PPpp")}
                />
              </ListItem>
            )}
            {scan.end_time && (
              <ListItem>
                <ListItemText
                  primary="Scan Completed"
                  secondary={format(new Date(scan.end_time), "PPpp")}
                />
              </ListItem>
            )}
            <ListItem>
              <ListItemText
                primary="Last Updated"
                secondary={format(new Date(scan.updated_at), "PPpp")}
              />
            </ListItem>
          </List>
        </TabPanel>
      </Card>

      {/* Delete confirmation dialog */}
      <Dialog open={deleteDialog} onClose={() => setDeleteDialog(false)} maxWidth="sm" fullWidth>
        <DialogTitle>Delete Scan</DialogTitle>
        <DialogContent>
          <Typography>
            Are you sure you want to delete the scan "{scan.name}"? This action cannot be undone.
          </Typography>
          {scan.status === "running" && (
            <Alert severity="warning" sx={{ mt: 2 }}>
              This scan is currently running. Deleting it will stop the scan immediately.
            </Alert>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setDeleteDialog(false)} disabled={mutations.delete.loading}>
            Cancel
          </Button>
          <Button
            onClick={handleDeleteScan}
            color="error"
            variant="contained"
            disabled={mutations.delete.loading}
          >
            {mutations.delete.loading ? "Deleting..." : "Delete"}
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
};

export default ScanDetail;
