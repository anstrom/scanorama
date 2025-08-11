import React, { useState, useCallback } from "react";
import { useNavigate } from "react-router-dom";
import {
  Box,
  Card,
  CardContent,
  Typography,
  Button,
  TextField,
  FormControl,
  InputLabel,
  Select,
  MenuItem,
  Grid,
  Alert,
  Paper,
  Chip,
  Stack,
  FormHelperText,
  Switch,
  FormControlLabel,
  Autocomplete,
  Stepper,
  Step,
  StepLabel,
  StepContent,
} from "@mui/material";
import {
  ArrowBack as BackIcon,
  Save as SaveIcon,
  PlayArrow as StartIcon,
  Help as HelpIcon,
} from "@mui/icons-material";

import { useScanMutations, useProfiles, useFormState } from "../../hooks/useApi";
import { ScanRequest, ScanType } from "../../types/api";

// Form validation
const validateScanForm = (values: ScanRequest): Record<string, string> => {
  const errors: Record<string, string> = {};

  if (!values.name?.trim()) {
    errors.name = "Scan name is required";
  } else if (values.name.length > 255) {
    errors.name = "Scan name must be less than 255 characters";
  }

  if (!values.targets || values.targets.length === 0) {
    errors.targets = "At least one target is required";
  } else {
    // Basic validation for IP addresses and ranges
    const invalidTargets = values.targets.filter((target) => {
      const ipRegex = /^(\d{1,3}\.){3}\d{1,3}(\/\d{1,2})?$/;
      const hostnameRegex = /^[a-zA-Z0-9.-]+$/;
      return !ipRegex.test(target.trim()) && !hostnameRegex.test(target.trim());
    });

    if (invalidTargets.length > 0) {
      errors.targets = `Invalid targets: ${invalidTargets.join(", ")}`;
    }
  }

  if (!values.scan_type) {
    errors.scan_type = "Scan type is required";
  }

  if (values.ports && values.ports.trim()) {
    // Validate port format (comma-separated numbers or ranges)
    const portRegex = /^(\d{1,5}(-\d{1,5})?,?\s*)*$/;
    if (!portRegex.test(values.ports.trim())) {
      errors.ports =
        "Invalid port format. Use comma-separated numbers or ranges (e.g., 80,443,8000-8100)";
    }
  }

  return errors;
};

// Initial form state
const initialFormState: ScanRequest = {
  name: "",
  description: "",
  targets: [],
  scan_type: "connect",
  ports: "",
  options: {},
  tags: [],
};

// Scan type descriptions
const scanTypeDescriptions: Record<ScanType, string> = {
  connect: "Basic TCP connect scan - reliable but detectable",
  syn: "SYN stealth scan - faster and less detectable",
  ack: "ACK scan - used to map firewall rulesets",
  aggressive: "Aggressive scan with OS detection and service enumeration",
  comprehensive: "Complete scan with all detection methods - slowest but most thorough",
  stealth: "Ultra-stealth scan designed to avoid detection",
};

// Common port presets
const portPresets = [
  { label: "Top 1000", value: "top-1000" },
  { label: "Common Web", value: "80,443,8000,8080,8443" },
  { label: "Common SSH/Remote", value: "22,23,3389,5985,5986" },
  { label: "Database Ports", value: "1433,1521,3306,5432,27017" },
  { label: "All Ports", value: "1-65535" },
  { label: "Custom", value: "" },
];

const NewScan: React.FC = () => {
  const navigate = useNavigate();
  const [activeStep, setActiveStep] = useState(0);
  const [startImmediately, setStartImmediately] = useState(false);
  const [targetInput, setTargetInput] = useState("");

  // API hooks
  const mutations = useScanMutations();
  const profiles = useProfiles();

  // Form state
  const form = useFormState(initialFormState, validateScanForm);

  // Event handlers
  const handleAddTarget = useCallback(() => {
    if (targetInput.trim()) {
      const newTargets = [...form.values.targets, targetInput.trim()];
      form.setValue("targets", newTargets);
      setTargetInput("");
    }
  }, [targetInput, form]);

  const handleRemoveTarget = useCallback(
    (index: number) => {
      const newTargets = form.values.targets.filter((_, i) => i !== index);
      form.setValue("targets", newTargets);
    },
    [form],
  );

  const handlePortPresetChange = useCallback(
    (preset: string) => {
      if (preset === "top-1000") {
        form.setValue("ports", ""); // API should handle top-1000 as default
      } else if (preset !== "") {
        form.setValue("ports", preset);
      }
    },
    [form],
  );

  const handleSubmit = useCallback(async () => {
    if (!form.validate()) {
      return;
    }

    const scanRequest: ScanRequest = {
      ...form.values,
    };

    // Only include options if they exist
    if (form.values.options && Object.keys(form.values.options).length > 0) {
      scanRequest.options = form.values.options;
    }

    const result = await mutations.create.execute(scanRequest);

    if (result) {
      if (startImmediately) {
        await mutations.start.execute(result.id);
      }
      navigate(`/scans/${result.id}`);
    }
  }, [form, mutations, startImmediately, navigate]);

  const handleNext = () => {
    setActiveStep((prevStep) => prevStep + 1);
  };

  const handleBack = () => {
    setActiveStep((prevStep) => prevStep - 1);
  };

  return (
    <Box>
      {/* Header */}
      <Box sx={{ mb: 4 }}>
        <Button startIcon={<BackIcon />} onClick={() => navigate("/scans")} sx={{ mb: 2 }}>
          Back to Scans
        </Button>
        <Typography variant="h4" component="h1" sx={{ fontWeight: "bold", mb: 1 }}>
          Create New Scan
        </Typography>
        <Typography variant="body1" color="text.secondary">
          Configure a new network scan to discover hosts and services
        </Typography>
      </Box>

      {/* Error display */}
      {mutations.create.error && (
        <Alert severity="error" sx={{ mb: 3 }}>
          Failed to create scan: {mutations.create.error.message}
        </Alert>
      )}

      <Grid container spacing={3}>
        {/* Main form */}
        <Grid item xs={12} md={8}>
          <Card>
            <CardContent>
              <Stepper activeStep={activeStep} orientation="vertical">
                {/* Step 1: Basic Information */}
                <Step>
                  <StepLabel>Basic Information</StepLabel>
                  <StepContent>
                    <Stack spacing={3}>
                      <TextField
                        fullWidth
                        label="Scan Name"
                        value={form.values.name}
                        onChange={(e) => form.setValue("name", e.target.value)}
                        error={!!form.errors.name}
                        helperText={form.errors.name}
                        placeholder="e.g., Weekly Infrastructure Scan"
                        required
                      />

                      <TextField
                        fullWidth
                        label="Description"
                        value={form.values.description}
                        onChange={(e) => form.setValue("description", e.target.value)}
                        placeholder="Optional description of this scan's purpose"
                        multiline
                        rows={3}
                      />

                      <FormControl fullWidth required error={!!form.errors.scan_type}>
                        <InputLabel>Scan Type</InputLabel>
                        <Select
                          value={form.values.scan_type}
                          label="Scan Type"
                          onChange={(e) => form.setValue("scan_type", e.target.value as ScanType)}
                        >
                          {Object.entries(scanTypeDescriptions).map(([type, description]) => (
                            <MenuItem key={type} value={type}>
                              <Box>
                                <Typography variant="body1">{type.toUpperCase()}</Typography>
                                <Typography variant="body2" color="text.secondary">
                                  {description}
                                </Typography>
                              </Box>
                            </MenuItem>
                          ))}
                        </Select>
                        {form.errors.scan_type && (
                          <FormHelperText>{form.errors.scan_type}</FormHelperText>
                        )}
                      </FormControl>

                      <Box sx={{ mt: 2 }}>
                        <Button variant="contained" onClick={handleNext}>
                          Next
                        </Button>
                      </Box>
                    </Stack>
                  </StepContent>
                </Step>

                {/* Step 2: Targets & Configuration */}
                <Step>
                  <StepLabel>Targets & Configuration</StepLabel>
                  <StepContent>
                    <Stack spacing={3}>
                      {/* Targets */}
                      <Box>
                        <Typography variant="h6" sx={{ mb: 2 }}>
                          Scan Targets
                        </Typography>
                        <Box sx={{ display: "flex", gap: 1, mb: 2 }}>
                          <TextField
                            fullWidth
                            label="Add Target"
                            value={targetInput}
                            onChange={(e) => setTargetInput(e.target.value)}
                            placeholder="IP address, hostname, or CIDR range"
                            onKeyPress={(e) => e.key === "Enter" && handleAddTarget()}
                            error={!!form.errors.targets}
                            helperText={form.errors.targets}
                          />
                          <Button
                            variant="outlined"
                            onClick={handleAddTarget}
                            disabled={!targetInput.trim()}
                            sx={{ minWidth: 100 }}
                          >
                            Add
                          </Button>
                        </Box>

                        {form.values.targets.length > 0 && (
                          <Paper sx={{ p: 2, mb: 2 }}>
                            <Typography variant="subtitle2" sx={{ mb: 1 }}>
                              Targets ({form.values.targets.length}):
                            </Typography>
                            <Box sx={{ display: "flex", flexWrap: "wrap", gap: 1 }}>
                              {form.values.targets.map((target, index) => (
                                <Chip
                                  key={index}
                                  label={target}
                                  onDelete={() => handleRemoveTarget(index)}
                                  variant="outlined"
                                />
                              ))}
                            </Box>
                          </Paper>
                        )}
                      </Box>

                      {/* Port configuration */}
                      <Box>
                        <Typography variant="h6" sx={{ mb: 2 }}>
                          Port Configuration
                        </Typography>
                        <FormControl fullWidth sx={{ mb: 2 }}>
                          <InputLabel>Port Preset</InputLabel>
                          <Select
                            label="Port Preset"
                            onChange={(e) => handlePortPresetChange(e.target.value)}
                            defaultValue=""
                          >
                            {portPresets.map((preset) => (
                              <MenuItem key={preset.label} value={preset.value}>
                                {preset.label}
                              </MenuItem>
                            ))}
                          </Select>
                        </FormControl>

                        <TextField
                          fullWidth
                          label="Custom Ports"
                          value={form.values.ports}
                          onChange={(e) => form.setValue("ports", e.target.value)}
                          placeholder="80,443,8000-8100 or leave empty for default"
                          error={!!form.errors.ports}
                          helperText={
                            form.errors.ports ||
                            "Comma-separated ports or ranges (e.g., 80,443,8000-8100)"
                          }
                        />
                      </Box>

                      {/* Profile selection */}
                      {profiles && profiles.data.length > 0 && (
                        <FormControl fullWidth>
                          <InputLabel>Scan Profile (Optional)</InputLabel>
                          <Select
                            value={form.values.profile_id || ""}
                            label="Scan Profile (Optional)"
                            onChange={(e) =>
                              form.setValue(
                                "profile_id",
                                e.target.value ? Number(e.target.value) : undefined,
                              )
                            }
                          >
                            <MenuItem value="">None</MenuItem>
                            {profiles.data.map((profile) => (
                              <MenuItem key={profile.id} value={profile.id}>
                                <Box>
                                  <Typography variant="body1">{profile.name}</Typography>
                                  <Typography variant="body2" color="text.secondary">
                                    {profile.description}
                                  </Typography>
                                </Box>
                              </MenuItem>
                            ))}
                          </Select>
                        </FormControl>
                      )}

                      <Box sx={{ mt: 2, display: "flex", gap: 1 }}>
                        <Button onClick={handleBack}>Back</Button>
                        <Button variant="contained" onClick={handleNext}>
                          Next
                        </Button>
                      </Box>
                    </Stack>
                  </StepContent>
                </Step>

                {/* Step 3: Advanced Options */}
                <Step>
                  <StepLabel>Advanced Options</StepLabel>
                  <StepContent>
                    <Stack spacing={3}>
                      {/* Tags */}
                      <Box>
                        <Typography variant="h6" sx={{ mb: 2 }}>
                          Tags (Optional)
                        </Typography>
                        <Autocomplete
                          multiple
                          freeSolo
                          options={[]}
                          value={form.values.tags || []}
                          onChange={(_, newValue) => form.setValue("tags", newValue)}
                          renderTags={(value, getTagProps) =>
                            value.map((option, index) => (
                              <Chip
                                variant="outlined"
                                label={option}
                                {...getTagProps({ index })}
                                key={index}
                              />
                            ))
                          }
                          renderInput={(params) => {
                            const { size, InputLabelProps, ...restParams } = params;
                            return (
                              <TextField
                                {...restParams}
                                InputLabelProps={
                                  InputLabelProps
                                    ? {
                                        ...Object.fromEntries(
                                          Object.entries(InputLabelProps).filter(
                                            ([, value]) => value !== undefined,
                                          ),
                                        ),
                                        className: InputLabelProps.className || "",
                                      }
                                    : { className: "" }
                                }
                                label="Tags"
                                placeholder="Add tags for organization"
                                helperText="Press Enter to add a tag"
                                size="small"
                              />
                            );
                          }}
                        />
                      </Box>

                      {/* Advanced options */}
                      <Box>
                        <Typography variant="h6" sx={{ mb: 2 }}>
                          Scan Options
                        </Typography>

                        <FormControlLabel
                          control={
                            <Switch
                              checked={startImmediately}
                              onChange={(e) => setStartImmediately(e.target.checked)}
                            />
                          }
                          label="Start scan immediately after creation"
                        />
                      </Box>

                      <Box sx={{ mt: 3, display: "flex", gap: 1 }}>
                        <Button onClick={handleBack}>Back</Button>
                        <Button
                          variant="contained"
                          onClick={handleSubmit}
                          disabled={mutations.create.loading || !form.isValid}
                          startIcon={startImmediately ? <StartIcon /> : <SaveIcon />}
                        >
                          {mutations.create.loading
                            ? "Creating..."
                            : startImmediately
                              ? "Create & Start"
                              : "Create Scan"}
                        </Button>
                      </Box>
                    </Stack>
                  </StepContent>
                </Step>
              </Stepper>
            </CardContent>
          </Card>
        </Grid>

        {/* Sidebar with help and tips */}
        <Grid item xs={12} md={4}>
          <Card>
            <CardContent>
              <Box sx={{ display: "flex", alignItems: "center", mb: 2 }}>
                <HelpIcon sx={{ mr: 1, color: "primary.main" }} />
                <Typography variant="h6">Quick Tips</Typography>
              </Box>

              <Stack spacing={2}>
                <Paper
                  sx={{
                    p: 2,
                    bgcolor: "info.light",
                    color: "info.contrastText",
                  }}
                >
                  <Typography variant="subtitle2" sx={{ fontWeight: "bold", mb: 1 }}>
                    Target Formats
                  </Typography>
                  <Typography variant="body2" sx={{ mb: 1 }}>
                    • Single IP: 192.168.1.100
                  </Typography>
                  <Typography variant="body2" sx={{ mb: 1 }}>
                    • IP Range: 192.168.1.1-192.168.1.100
                  </Typography>
                  <Typography variant="body2" sx={{ mb: 1 }}>
                    • CIDR: 192.168.1.0/24
                  </Typography>
                  <Typography variant="body2">• Hostname: example.com</Typography>
                </Paper>

                <Paper
                  sx={{
                    p: 2,
                    bgcolor: "warning.light",
                    color: "warning.contrastText",
                  }}
                >
                  <Typography variant="subtitle2" sx={{ fontWeight: "bold", mb: 1 }}>
                    Scan Types
                  </Typography>
                  <Typography variant="body2" sx={{ mb: 1 }}>
                    • <strong>Connect:</strong> Most reliable, slower
                  </Typography>
                  <Typography variant="body2" sx={{ mb: 1 }}>
                    • <strong>SYN:</strong> Faster, less detectable
                  </Typography>
                  <Typography variant="body2">
                    • <strong>Aggressive:</strong> Most comprehensive
                  </Typography>
                </Paper>

                <Paper
                  sx={{
                    p: 2,
                    bgcolor: "success.light",
                    color: "success.contrastText",
                  }}
                >
                  <Typography variant="subtitle2" sx={{ fontWeight: "bold", mb: 1 }}>
                    Best Practices
                  </Typography>
                  <Typography variant="body2" sx={{ mb: 1 }}>
                    • Start with connect scans for reliability
                  </Typography>
                  <Typography variant="body2" sx={{ mb: 1 }}>
                    • Use descriptive names and tags
                  </Typography>
                  <Typography variant="body2">• Test on small ranges first</Typography>
                </Paper>
              </Stack>
            </CardContent>
          </Card>

          {/* Current configuration preview */}
          <Card sx={{ mt: 3 }}>
            <CardContent>
              <Typography variant="h6" sx={{ mb: 2 }}>
                Configuration Preview
              </Typography>
              <Stack spacing={1}>
                <Box>
                  <Typography variant="body2" color="text.secondary">
                    Name:
                  </Typography>
                  <Typography variant="body1">{form.values.name || "Untitled Scan"}</Typography>
                </Box>
                <Box>
                  <Typography variant="body2" color="text.secondary">
                    Type:
                  </Typography>
                  <Typography variant="body1">{form.values.scan_type.toUpperCase()}</Typography>
                </Box>
                <Box>
                  <Typography variant="body2" color="text.secondary">
                    Targets:
                  </Typography>
                  <Typography variant="body1">
                    {form.values.targets.length || 0} target
                    {form.values.targets.length !== 1 ? "s" : ""}
                  </Typography>
                </Box>
                <Box>
                  <Typography variant="body2" color="text.secondary">
                    Ports:
                  </Typography>
                  <Typography variant="body1">{form.values.ports || "Default ports"}</Typography>
                </Box>
                {form.values.tags && form.values.tags.length > 0 && (
                  <Box>
                    <Typography variant="body2" color="text.secondary">
                      Tags:
                    </Typography>
                    <Box sx={{ mt: 1 }}>
                      {form.values.tags.map((tag) => (
                        <Chip key={tag} label={tag} size="small" sx={{ mr: 0.5, mb: 0.5 }} />
                      ))}
                    </Box>
                  </Box>
                )}
              </Stack>
            </CardContent>
          </Card>
        </Grid>
      </Grid>
    </Box>
  );
};

export default NewScan;
