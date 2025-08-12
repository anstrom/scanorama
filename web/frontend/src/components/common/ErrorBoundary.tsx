import { Component, ReactNode, ErrorInfo } from "react";
import { Box, Button, Card, CardContent, Typography, Alert, Collapse, Stack } from "@mui/material";
import {
  Error as ErrorIcon,
  Refresh as RefreshIcon,
  ExpandMore as ExpandMoreIcon,
  BugReport as BugReportIcon,
  Home as HomeIcon,
} from "@mui/icons-material";
import logger from "../../utils/logger";

interface Props {
  children: ReactNode;
  fallback?: ReactNode;
  onError?: (error: Error, errorInfo: ErrorInfo) => void;
}

interface State {
  hasError: boolean;
  error: Error | null;
  errorInfo: ErrorInfo | null;
  showDetails: boolean;
}

class ErrorBoundary extends Component<Props, State> {
  constructor(props: Props) {
    super(props);
    this.state = {
      hasError: false,
      error: null,
      errorInfo: null,
      showDetails: false,
    };
  }

  static getDerivedStateFromError(error: Error): Partial<State> {
    // Update state so the next render will show the fallback UI
    return {
      hasError: true,
      error,
    };
  }

  override componentDidCatch(error: Error, errorInfo: ErrorInfo) {
    // Log the error with comprehensive context
    logger.error("React ErrorBoundary caught an error", {
      context: {
        component: "ErrorBoundary",
        action: "componentDidCatch",
      },
      error,
      metadata: {
        componentStack: errorInfo.componentStack,
      },
    });

    // Update state with error info
    this.setState({
      error,
      errorInfo,
    });

    // Call the onError prop if provided
    if (this.props.onError) {
      this.props.onError(error, errorInfo);
    }
  }

  handleRetry = () => {
    logger.info("User attempting error recovery", {
      component: "ErrorBoundary",
      action: "retry",
    });

    this.setState({
      hasError: false,
      error: null,
      errorInfo: null,
      showDetails: false,
    });
  };

  handleToggleDetails = () => {
    this.setState((prevState) => ({
      showDetails: !prevState.showDetails,
    }));
  };

  handleGoHome = () => {
    logger.info("User navigating to dashboard from error state", {
      component: "ErrorBoundary",
      action: "go_home",
    });

    window.location.href = "/dashboard";
  };

  override render() {
    if (this.state.hasError) {
      // Custom fallback UI
      if (this.props.fallback) {
        return this.props.fallback;
      }

      // Default fallback UI
      return (
        <Box
          sx={{
            display: "flex",
            justifyContent: "center",
            alignItems: "center",
            minHeight: "50vh",
            p: 3,
          }}
        >
          <Card sx={{ maxWidth: 600, width: "100%" }}>
            <CardContent sx={{ textAlign: "center", p: 4 }}>
              {/* Error icon */}
              <ErrorIcon
                sx={{
                  fontSize: 64,
                  color: "error.main",
                  mb: 2,
                }}
              />

              {/* Error title */}
              <Typography variant="h4" component="h1" sx={{ mb: 2, fontWeight: "bold" }}>
                Oops! Something went wrong
              </Typography>

              {/* Error description */}
              <Typography variant="body1" color="text.secondary" sx={{ mb: 3 }}>
                We encountered an unexpected error. This has been logged and we'll look into it. You
                can try refreshing the page or return to the dashboard.
              </Typography>

              {/* Action buttons */}
              <Stack direction="row" spacing={2} justifyContent="center" sx={{ mb: 3 }}>
                <Button
                  variant="contained"
                  startIcon={<RefreshIcon />}
                  onClick={this.handleRetry}
                  size="large"
                >
                  Try Again
                </Button>
                <Button
                  variant="outlined"
                  startIcon={<HomeIcon />}
                  onClick={this.handleGoHome}
                  size="large"
                >
                  Go to Dashboard
                </Button>
              </Stack>

              {/* Error details toggle */}
              {process.env.NODE_ENV === "development" && this.state.error && (
                <Box>
                  <Button
                    variant="text"
                    startIcon={<BugReportIcon />}
                    endIcon={
                      <ExpandMoreIcon
                        sx={{
                          transform: this.state.showDetails ? "rotate(180deg)" : "rotate(0deg)",
                          transition: "transform 0.3s",
                        }}
                      />
                    }
                    onClick={this.handleToggleDetails}
                    size="small"
                    color="inherit"
                  >
                    {this.state.showDetails ? "Hide" : "Show"} Error Details
                  </Button>

                  <Collapse in={this.state.showDetails}>
                    <Box sx={{ mt: 2, textAlign: "left" }}>
                      <Alert severity="error" sx={{ mb: 2 }}>
                        <Typography variant="subtitle2" sx={{ fontWeight: "bold", mb: 1 }}>
                          Error Message:
                        </Typography>
                        <Typography
                          variant="body2"
                          component="pre"
                          sx={{ fontFamily: "monospace" }}
                        >
                          {this.state.error.message}
                        </Typography>
                      </Alert>

                      {this.state.error.stack && (
                        <Alert severity="warning">
                          <Typography variant="subtitle2" sx={{ fontWeight: "bold", mb: 1 }}>
                            Stack Trace:
                          </Typography>
                          <Typography
                            variant="body2"
                            component="pre"
                            sx={{
                              fontFamily: "monospace",
                              fontSize: "0.75rem",
                              whiteSpace: "pre-wrap",
                              wordBreak: "break-all",
                              maxHeight: 200,
                              overflow: "auto",
                            }}
                          >
                            {this.state.error.stack}
                          </Typography>
                        </Alert>
                      )}

                      {this.state.errorInfo?.componentStack && (
                        <Alert severity="info" sx={{ mt: 2 }}>
                          <Typography variant="subtitle2" sx={{ fontWeight: "bold", mb: 1 }}>
                            Component Stack:
                          </Typography>
                          <Typography
                            variant="body2"
                            component="pre"
                            sx={{
                              fontFamily: "monospace",
                              fontSize: "0.75rem",
                              whiteSpace: "pre-wrap",
                              wordBreak: "break-all",
                              maxHeight: 200,
                              overflow: "auto",
                            }}
                          >
                            {this.state.errorInfo.componentStack}
                          </Typography>
                        </Alert>
                      )}
                    </Box>
                  </Collapse>
                </Box>
              )}

              {/* Additional help text */}
              <Typography variant="caption" color="text.secondary" sx={{ mt: 2, display: "block" }}>
                If this problem persists, please check the browser console for more details or
                contact your system administrator.
              </Typography>
            </CardContent>
          </Card>
        </Box>
      );
    }

    // Render children if no error
    return this.props.children;
  }
}

export default ErrorBoundary;
