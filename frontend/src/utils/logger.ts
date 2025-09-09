/**
 * Professional logging service with structured logging capabilities
 * Provides consistent logging interface across the application
 */

// ============================================================================
// TYPES
// ============================================================================

export type LogLevel = 'debug' | 'info' | 'warn' | 'error';

export interface LogContext {
  readonly [key: string]: unknown;
}

export interface LogEntry {
  readonly level: LogLevel;
  readonly message: string;
  readonly timestamp: string;
  readonly context?: LogContext;
  readonly error?: Error;
}

export interface LoggerConfig {
  readonly level: LogLevel;
  readonly enabled: boolean;
  readonly includeTimestamp: boolean;
  readonly includeStackTrace: boolean;
  readonly maxContextDepth: number;
}

export interface LoggerTransport {
  log(entry: LogEntry): void;
}

// ============================================================================
// DEFAULT CONFIGURATION
// ============================================================================

const DEFAULT_CONFIG: LoggerConfig = {
  level: import.meta.env.DEV ? 'debug' : 'warn',
  enabled: true,
  includeTimestamp: true,
  includeStackTrace: import.meta.env.DEV,
  maxContextDepth: 3,
};

// ============================================================================
// LOG LEVELS
// ============================================================================

const LOG_LEVELS: Record<LogLevel, number> = {
  debug: 0,
  info: 1,
  warn: 2,
  error: 3,
} as const;

// ============================================================================
// CONSOLE TRANSPORT
// ============================================================================

class ConsoleTransport implements LoggerTransport {
  private readonly colors = {
    debug: 'color: #6b7280',
    info: 'color: #3b82f6',
    warn: 'color: #f59e0b',
    error: 'color: #ef4444',
  } as const;

  log(entry: LogEntry): void {
    const { level, message, timestamp, context, error } = entry;
    const color = this.colors[level];

    const parts: unknown[] = [
      `%c[${level.toUpperCase()}]`,
      color,
      timestamp,
      message,
    ];

    if (context && Object.keys(context).length > 0) {
      parts.push('\nContext:', context);
    }

    if (error) {
      parts.push('\nError:', error);
    }

    switch (level) {
      case 'debug':
        // eslint-disable-next-line no-console
        console.debug(...parts);
        break;
      case 'info':
        // eslint-disable-next-line no-console
        console.info(...parts);
        break;
      case 'warn':
        // eslint-disable-next-line no-console
        console.warn(...parts);
        break;
      case 'error':
        // eslint-disable-next-line no-console
        console.error(...parts);
        break;
      default:
        // eslint-disable-next-line no-console
        console.log(...parts);
    }
  }
}

// ============================================================================
// LOGGER CLASS
// ============================================================================

export class Logger {
  private readonly config: LoggerConfig;
  private readonly transports: LoggerTransport[];
  private readonly contextStack: LogContext[];

  constructor(
    config: Partial<LoggerConfig> = {},
    transports: LoggerTransport[] = [new ConsoleTransport()]
  ) {
    this.config = { ...DEFAULT_CONFIG, ...config };
    this.transports = transports;
    this.contextStack = [];
  }

  /**
   * Check if a log level should be processed
   */
  private shouldLog(level: LogLevel): boolean {
    if (!this.config.enabled) {
      return false;
    }
    return LOG_LEVELS[level] >= LOG_LEVELS[this.config.level];
  }

  /**
   * Safely serialize context data
   */
  private serializeContext(context: LogContext): LogContext {
    try {
      const serialized = JSON.parse(JSON.stringify(context)) as LogContext;
      return this.limitDepth(serialized, this.config.maxContextDepth);
    } catch {
      return { contextSerializationError: 'Failed to serialize context' };
    }
  }

  /**
   * Limit object depth to prevent circular references
   */
  private limitDepth(obj: unknown, depth: number): LogContext {
    if (depth <= 0) {
      return { value: '[Max depth reached]' };
    }

    if (obj === null || typeof obj !== 'object') {
      return { value: obj };
    }

    if (Array.isArray(obj)) {
      return {
        array: obj.map((item) => this.limitDepth(item, depth - 1))
      };
    }

    const result: Record<string, unknown> = {};
    for (const [key, value] of Object.entries(obj)) {
      result[key] = this.limitDepth(value, depth - 1);
    }
    return result;
  }

  /**
   * Merge context from stack and provided context
   */
  private mergeContext(context?: LogContext): LogContext | undefined {
    const stackContext = this.contextStack.reduce((acc, ctx) => ({ ...acc, ...ctx }), {});
    const mergedContext = { ...stackContext, ...context };

    return Object.keys(mergedContext).length > 0
      ? this.serializeContext(mergedContext)
      : undefined;
  }

  /**
   * Create a log entry
   */
  private createLogEntry(
    level: LogLevel,
    message: string,
    context?: LogContext,
    error?: Error
  ): LogEntry {
    const mergedContext = this.mergeContext(context);
    const entry: LogEntry = {
      level,
      message,
      timestamp: this.config.includeTimestamp
        ? new Date().toISOString()
        : '',
      ...(mergedContext && { context: mergedContext }),
      ...(error && this.config.includeStackTrace && { error }),
    };
    return entry;
  }

  /**
   * Log a message at the specified level
   */
  private log(
    level: LogLevel,
    message: string,
    context?: LogContext,
    error?: Error
  ): void {
    if (!this.shouldLog(level)) {
      return;
    }

    const entry = this.createLogEntry(level, message, context, error);

    for (const transport of this.transports) {
      try {
        transport.log(entry);
      } catch (transportError) {
        // Fallback to console if transport fails
        // eslint-disable-next-line no-console
        console.error('Logger transport failed:', transportError);
        // eslint-disable-next-line no-console
        console.error('Original log entry:', entry);
      }
    }
  }

  /**
   * Add context that will be included in all subsequent log messages
   */
  pushContext(context: LogContext): void {
    this.contextStack.push(context);
  }

  /**
   * Remove the most recent context from the stack
   */
  popContext(): LogContext | undefined {
    return this.contextStack.pop();
  }

  /**
   * Clear all context from the stack
   */
  clearContext(): void {
    this.contextStack.length = 0;
  }

  /**
   * Create a child logger with persistent context
   */
  child(context: LogContext): Logger {
    const childLogger = new Logger(this.config, this.transports);
    childLogger.contextStack.push(...this.contextStack, context);
    return childLogger;
  }

  /**
   * Log debug message
   */
  debug(message: string, context?: LogContext): void {
    this.log('debug', message, context);
  }

  /**
   * Log info message
   */
  info(message: string, context?: LogContext): void {
    this.log('info', message, context);
  }

  /**
   * Log warning message
   */
  warn(message: string, context?: LogContext, error?: Error): void {
    this.log('warn', message, context, error);
  }

  /**
   * Log error message
   */
  error(message: string, context?: LogContext, error?: Error): void {
    this.log('error', message, context, error);
  }

  /**
   * Log API request
   */
  apiRequest(method: string, url: string, context?: LogContext): void {
    this.debug(`API Request: ${method} ${url}`, {
      type: 'api_request',
      method,
      url,
      ...context,
    });
  }

  /**
   * Log API response
   */
  apiResponse(
    method: string,
    url: string,
    status: number,
    duration: number,
    context?: LogContext
  ): void {
    const level = status >= 400 ? 'error' : status >= 300 ? 'warn' : 'debug';
    this.log(level, `API Response: ${method} ${url} - ${status} (${duration}ms)`, {
      type: 'api_response',
      method,
      url,
      status,
      duration,
      ...context,
    });
  }

  /**
   * Log WebSocket events
   */
  websocket(event: string, context?: LogContext): void {
    this.debug(`WebSocket: ${event}`, {
      type: 'websocket',
      event,
      ...context,
    });
  }

  /**
   * Log user interactions
   */
  userAction(action: string, context?: LogContext): void {
    this.info(`User Action: ${action}`, {
      type: 'user_action',
      action,
      ...context,
    });
  }

  /**
   * Log performance metrics
   */
  performance(operation: string, duration: number, context?: LogContext): void {
    const level = duration > 1000 ? 'warn' : 'debug';
    this.log(level, `Performance: ${operation} took ${duration}ms`, {
      type: 'performance',
      operation,
      duration,
      ...context,
    });
  }
}

// ============================================================================
// SINGLETON INSTANCE
// ============================================================================

export const logger = new Logger();

// ============================================================================
// UTILITY FUNCTIONS
// ============================================================================

/**
 * Create a scoped logger with persistent context
 */
export const createScopedLogger = (scope: string, context?: LogContext): Logger => {
  return logger.child({ scope, ...context });
};

/**
 * Measure execution time of a function
 */
export const measureTime = async <T>(
  operation: string,
  fn: () => Promise<T> | T,
  context?: LogContext
): Promise<T> => {
  const start = performance.now();
  try {
    const result = await fn();
    const duration = performance.now() - start;
    logger.performance(operation, duration, context);
    return result;
  } catch (error) {
    const duration = performance.now() - start;
    logger.error(`${operation} failed after ${duration}ms`, context, error as Error);
    throw error;
  }
};

/**
 * Create a logger for specific component or service
 */
export const createLogger = (name: string): Logger => {
  return createScopedLogger(name);
};

// ============================================================================
// EXPORTS
// ============================================================================

export default logger;
