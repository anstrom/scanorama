export enum LogLevel {
  DEBUG = 0,
  INFO = 1,
  WARN = 2,
  ERROR = 3,
}

export interface LogContext {
  userId?: string;
  sessionId?: string;
  component?: string;
  action?: string;
  url?: string;
  userAgent?: string;
  timestamp?: string;
  buildVersion?: string;
  environment?: string;
  endpoint?: string;
  method?: string;
  status?: number;
}

export interface LogEntry {
  level: LogLevel;
  message: string;
  context?: LogContext | undefined;
  error?: Error | undefined;
  metadata?: Record<string, any> | undefined;
  timestamp: string;
  id: string;
}

export interface LoggerConfig {
  level: LogLevel;
  enableConsole: boolean;
  enableRemote: boolean;
  remoteEndpoint?: string;
  apiKey?: string;
  batchSize: number;
  flushInterval: number;
  maxRetries: number;
  enableErrorReporting: boolean;
}

class Logger {
  private config: LoggerConfig;
  private logBuffer: LogEntry[] = [];
  private flushTimer?: NodeJS.Timeout;
  private retryCount = 0;
  private sessionId: string;
  private dedupCache = new Set<string>();

  constructor(config: Partial<LoggerConfig> = {}) {
    this.config = {
      level: process.env.NODE_ENV === "production" ? LogLevel.WARN : LogLevel.DEBUG,
      enableConsole: true,
      enableRemote: process.env.NODE_ENV === "production",
      batchSize: 50,
      flushInterval: 30000, // 30 seconds
      maxRetries: 3,
      enableErrorReporting: true,
      ...config,
    };

    this.sessionId = this.generateSessionId();
    this.setupPeriodicFlush();
    this.setupUnloadHandler();
    this.setupErrorHandlers();
  }

  private generateSessionId(): string {
    return `session_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`;
  }

  private generateLogId(): string {
    return `log_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`;
  }

  private setupPeriodicFlush(): void {
    if (this.config.enableRemote) {
      this.flushTimer = setInterval(() => {
        this.flush();
      }, this.config.flushInterval);
    }
  }

  private setupUnloadHandler(): void {
    if (typeof window !== "undefined") {
      window.addEventListener("beforeunload", () => {
        this.flush();
      });

      document.addEventListener("visibilitychange", () => {
        if (document.visibilityState === "hidden") {
          this.flush();
        }
      });
    }
  }

  private setupErrorHandlers(): void {
    if (typeof window !== "undefined" && this.config.enableErrorReporting) {
      window.addEventListener("error", (event) => {
        this.error("Unhandled JavaScript error", {
          context: {
            component: "Global",
            action: "unhandled_error",
            url: window.location.href,
          },
          error: new Error(event.message),
          metadata: {
            filename: event.filename,
            lineno: event.lineno,
            colno: event.colno,
          },
        });
      });

      window.addEventListener("unhandledrejection", (event) => {
        this.error("Unhandled promise rejection", {
          context: {
            component: "Global",
            action: "unhandled_rejection",
            url: window.location.href,
          },
          error: event.reason instanceof Error ? event.reason : new Error(String(event.reason)),
        });
      });
    }
  }

  private getBaseContext(): LogContext {
    const context: LogContext = {
      sessionId: this.sessionId,
      timestamp: new Date().toISOString(),
      buildVersion: process.env.REACT_APP_VERSION || "unknown",
      environment: process.env.NODE_ENV || "development",
    };

    if (typeof window !== "undefined") {
      context.url = window.location.href;
    }

    if (typeof navigator !== "undefined") {
      context.userAgent = navigator.userAgent;
    }

    return context;
  }

  private shouldLog(level: LogLevel): boolean {
    return level >= this.config.level;
  }

  private createLogEntry(
    level: LogLevel,
    message: string,
    options: {
      context?: LogContext;
      error?: Error;
      metadata?: Record<string, any>;
    } = {},
  ): LogEntry {
    const baseContext = this.getBaseContext();
    const fullContext = { ...baseContext, ...options.context };

    return {
      id: this.generateLogId(),
      level,
      message,
      context: fullContext,
      error: options.error || undefined,
      metadata: options.metadata || undefined,
      timestamp: new Date().toISOString(),
    };
  }

  private logToConsole(entry: LogEntry): void {
    if (!this.config.enableConsole) return;

    const levelNames = ["DEBUG", "INFO", "WARN", "ERROR"];
    const methods = ["log", "info", "warn", "error"] as const;

    const method = methods[entry.level] || "log";
    const prefix = `[${entry.timestamp}] [${levelNames[entry.level]}]`;
    const contextStr = entry.context?.component
      ? ` [${entry.context.component}${entry.context.action ? `:${entry.context.action}` : ""}]`
      : "";

    if (entry.error) {
      console[method](`${prefix}${contextStr} ${entry.message}`, entry.error, entry.metadata);
    } else {
      console[method](`${prefix}${contextStr} ${entry.message}`, entry.metadata);
    }
  }

  private shouldDeduplicate(entry: LogEntry): boolean {
    const key = `${entry.level}_${entry.message}_${entry.context?.component}_${entry.context?.action}`;
    if (this.dedupCache.has(key)) {
      return true;
    }

    this.dedupCache.add(key);

    if (this.dedupCache.size > 1000) {
      const entries = Array.from(this.dedupCache);
      this.dedupCache.clear();
      entries.slice(-500).forEach((entry) => this.dedupCache.add(entry));
    }

    return false;
  }

  private addToBuffer(entry: LogEntry): void {
    if (!this.config.enableRemote) return;

    if (entry.level < LogLevel.ERROR && this.shouldDeduplicate(entry)) {
      return;
    }

    this.logBuffer.push(entry);

    if (this.logBuffer.length >= this.config.batchSize || entry.level >= LogLevel.ERROR) {
      this.flush();
    }
  }

  private async sendLogs(entries: LogEntry[]): Promise<boolean> {
    if (!this.config.remoteEndpoint || entries.length === 0) {
      return false;
    }

    try {
      const response = await fetch(this.config.remoteEndpoint, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          ...(this.config.apiKey && {
            Authorization: `Bearer ${this.config.apiKey}`,
          }),
        },
        body: JSON.stringify({
          logs: entries,
          session_id: this.sessionId,
          client: "scanorama-frontend",
        }),
      });

      if (!response.ok) {
        throw new Error(`HTTP ${response.status}: ${response.statusText}`);
      }

      this.retryCount = 0;
      return true;
    } catch (error) {
      this.retryCount++;
      console.warn(`Failed to send logs (attempt ${this.retryCount}):`, error);

      if (this.retryCount >= this.config.maxRetries) {
        console.error("Max retries exceeded for log transmission, dropping logs");
        this.retryCount = 0;
        return false;
      }

      return false;
    }
  }

  public async flush(): Promise<void> {
    if (this.logBuffer.length === 0) return;

    const logsToSend = [...this.logBuffer];
    this.logBuffer = [];

    const success = await this.sendLogs(logsToSend);

    if (!success && this.retryCount < this.config.maxRetries) {
      this.logBuffer.unshift(...logsToSend);
    }
  }

  public debug(message: string, context?: LogContext, metadata?: Record<string, any>): void {
    if (!this.shouldLog(LogLevel.DEBUG)) return;

    const options: any = {};
    if (context) options.context = context;
    if (metadata) options.metadata = metadata;

    const entry = this.createLogEntry(LogLevel.DEBUG, message, options);
    this.logToConsole(entry);
    this.addToBuffer(entry);
  }

  public info(message: string, context?: LogContext, metadata?: Record<string, any>): void {
    if (!this.shouldLog(LogLevel.INFO)) return;

    const options: any = {};
    if (context) options.context = context;
    if (metadata) options.metadata = metadata;

    const entry = this.createLogEntry(LogLevel.INFO, message, options);
    this.logToConsole(entry);
    this.addToBuffer(entry);
  }

  public warn(message: string, context?: LogContext, metadata?: Record<string, any>): void {
    if (!this.shouldLog(LogLevel.WARN)) return;

    const options: any = {};
    if (context) options.context = context;
    if (metadata) options.metadata = metadata;

    const entry = this.createLogEntry(LogLevel.WARN, message, options);
    this.logToConsole(entry);
    this.addToBuffer(entry);
  }

  public error(
    message: string,
    options: {
      context?: LogContext;
      error?: Error;
      metadata?: Record<string, any>;
    } = {},
  ): void {
    if (!this.shouldLog(LogLevel.ERROR)) return;

    const entry = this.createLogEntry(LogLevel.ERROR, message, options);
    this.logToConsole(entry);
    this.addToBuffer(entry);

    if (this.config.enableRemote) {
      this.flush();
    }
  }

  public logUserAction(action: string, component: string, metadata?: Record<string, any>): void {
    this.info(`User action: ${action}`, { component, action }, metadata);
  }

  public logPerformance(
    operation: string,
    duration: number,
    component?: string,
    metadata?: Record<string, any>,
  ): void {
    this.info(
      `Performance: ${operation} took ${duration}ms`,
      { component: component || "Performance", action: operation },
      { duration, ...metadata },
    );
  }

  public setUserId(userId: string): void {
    (this as any).userId = userId;
  }

  public setContext(context: LogContext): void {
    (this as any).defaultContext = {
      ...(this as any).defaultContext,
      ...context,
    };
  }

  public getSessionId(): string {
    return this.sessionId;
  }

  public updateConfig(newConfig: Partial<LoggerConfig>): void {
    this.config = { ...this.config, ...newConfig };

    if (newConfig.flushInterval && this.flushTimer) {
      clearInterval(this.flushTimer);
      this.setupPeriodicFlush();
    }
  }

  public getStats(): {
    bufferSize: number;
    sessionId: string;
    retryCount: number;
    cacheSize: number;
  } {
    return {
      bufferSize: this.logBuffer.length,
      sessionId: this.sessionId,
      retryCount: this.retryCount,
      cacheSize: this.dedupCache.size,
    };
  }

  public async exportLogs(): Promise<LogEntry[]> {
    return [...this.logBuffer];
  }

  public clearLogs(): void {
    this.logBuffer = [];
    this.dedupCache.clear();
  }

  public destroy(): void {
    if (this.flushTimer) {
      clearInterval(this.flushTimer);
    }
    this.flush();
    this.clearLogs();
  }
}

// Create singleton logger instance
const logger = new Logger({
  ...(process.env.REACT_APP_LOG_ENDPOINT && {
    remoteEndpoint: process.env.REACT_APP_LOG_ENDPOINT,
  }),
  ...(process.env.REACT_APP_LOG_API_KEY && {
    apiKey: process.env.REACT_APP_LOG_API_KEY,
  }),
});

// Helper functions for common logging patterns
export const logApiCall = (endpoint: string, method: string, duration?: number) => {
  logger.info(
    `API call: ${method} ${endpoint}`,
    { component: "API", action: "request" },
    { endpoint, method, duration },
  );
};

export const logError = (message: string, error: Error, component?: string) => {
  const options: any = { error };
  if (component) {
    options.context = { component };
  }
  logger.error(message, options);
};

export const logUserAction = (
  action: string,
  component: string,
  metadata?: Record<string, any>,
) => {
  logger.logUserAction(action, component, metadata);
};

export const logPerformance = (operation: string, duration: number, component?: string) => {
  logger.logPerformance(operation, duration, component);
};

export default logger;
