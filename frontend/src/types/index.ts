// ============================================================================
// CORE API TYPES
// ============================================================================

/**
 * Health check response from the API
 */
export interface HealthResponse {
  readonly status: 'healthy' | 'unhealthy' | 'degraded';
  readonly checks: {
    readonly database: 'ok' | 'error';
    readonly [key: string]: string;
  };
  readonly timestamp: string;
}

/**
 * System status response from the API
 */
export interface StatusResponse {
  readonly service: string;
  readonly version: string;
  readonly uptime: string;
  readonly timestamp: string;
}

/**
 * Pagination metadata
 */
export interface PaginationInfo {
  readonly page: number;
  readonly page_size: number;
  readonly total_items: number;
  readonly total_pages: number;
}

/**
 * Generic paginated response wrapper
 */
export interface PaginatedResponse<T> {
  readonly data: readonly T[];
  readonly pagination: PaginationInfo;
}

// ============================================================================
// DOMAIN TYPES
// ============================================================================

/**
 * Scan profile configuration
 */
export interface Profile {
  readonly id: string;
  readonly name: string;
  readonly description: string;
  readonly scan_type: ScanType;
  readonly ports: string;
  readonly timing: {
    readonly template: TimingTemplate;
  };
  readonly service_detection: boolean;
  readonly os_detection: boolean;
  readonly script_scan: boolean;
  readonly udp_scan: boolean;
  readonly default: boolean;
  readonly usage_count: number;
  readonly created_at: string;
  readonly updated_at: string;
}

/**
 * Scan execution record
 */
export interface Scan {
  readonly id: string;
  readonly name?: string;
  readonly description?: string;
  readonly targets: readonly string[];
  readonly profile_id: string;
  readonly status: ScanStatus;
  readonly progress: number;
  readonly started_at?: string;
  readonly completed_at?: string;
  readonly duration?: number;
  readonly hosts_discovered: number;
  readonly ports_scanned: number;
  readonly error_message?: string;
  readonly created_at: string;
  readonly updated_at: string;
}

/**
 * Discovered host information
 */
export interface Host {
  readonly id: string;
  readonly ip_address: string;
  readonly hostname?: string;
  readonly mac_address?: string;
  readonly status: HostStatus;
  readonly first_seen: string;
  readonly last_seen: string;
  readonly scan_count: number;
  readonly open_ports: readonly number[];
}

/**
 * Network discovery job configuration
 */
export interface DiscoveryJob {
  readonly id: number;
  readonly name: string;
  readonly description?: string;
  readonly networks: readonly string[];
  readonly method: DiscoveryMethod;
  readonly enabled: boolean;
  readonly status: JobStatus;
  readonly progress: number;
  readonly hosts_found: number;
  readonly created_at: string;
  readonly updated_at: string;
}

// ============================================================================
// ENUMS AND UNION TYPES
// ============================================================================

/**
 * Available scan types
 */
export type ScanType =
  | 'connect'
  | 'syn'
  | 'version'
  | 'comprehensive'
  | 'aggressive'
  | 'stealth'
  | 'udp';

/**
 * Scan timing templates
 */
export type TimingTemplate =
  | 'paranoid'
  | 'sneaky'
  | 'polite'
  | 'normal'
  | 'aggressive'
  | 'insane';

/**
 * Scan execution status
 */
export type ScanStatus =
  | 'queued'
  | 'running'
  | 'completed'
  | 'failed'
  | 'cancelled'
  | 'paused';

/**
 * Host discovery status
 */
export type HostStatus =
  | 'up'
  | 'down'
  | 'unknown'
  | 'scanning';

/**
 * Discovery job status
 */
export type JobStatus =
  | 'active'
  | 'inactive'
  | 'running'
  | 'error';

/**
 * Network discovery methods
 */
export type DiscoveryMethod =
  | 'ping'
  | 'tcp'
  | 'udp'
  | 'arp'
  | 'icmp';

// ============================================================================
// WEBSOCKET TYPES
// ============================================================================

/**
 * WebSocket connection status
 */
export type ConnectionStatus =
  | 'disconnected'
  | 'connecting'
  | 'connected'
  | 'error'
  | 'reconnecting';

/**
 * Base WebSocket message structure
 */
export interface WebSocketMessage<T = unknown> {
  readonly type: string;
  readonly payload: T;
  readonly timestamp: string;
}

/**
 * Scan progress WebSocket message
 */
export interface ScanProgressMessage extends WebSocketMessage<{
  readonly scanId: string;
  readonly progress: number;
  readonly status: ScanStatus;
  readonly message?: string;
}> {
  readonly type: 'scan_progress';
}

/**
 * Host discovered WebSocket message
 */
export interface HostDiscoveredMessage extends WebSocketMessage<{
  readonly host: Host;
  readonly scanId?: string;
}> {
  readonly type: 'host_discovered';
}

/**
 * System status update WebSocket message
 */
export interface SystemStatusMessage extends WebSocketMessage<{
  readonly status: HealthResponse['status'];
  readonly timestamp: string;
}> {
  readonly type: 'system_status';
}

/**
 * WebSocket message handler function
 */
export type MessageHandler<T = unknown> = (message: WebSocketMessage<T>) => void;

// ============================================================================
// ERROR TYPES
// ============================================================================

/**
 * API error response
 */
export interface ApiError {
  readonly code: string;
  readonly message: string;
  readonly details?: Record<string, unknown>;
  readonly timestamp: string;
  readonly requestId?: string;
}

/**
 * Application error with context
 */
export interface AppError extends Error {
  readonly code: string;
  readonly context?: Record<string, unknown>;
  readonly timestamp: Date;
}

// ============================================================================
// UI COMPONENT TYPES
// ============================================================================

/**
 * Common component props
 */
export interface BaseComponentProps {
  readonly className?: string;
  readonly testId?: string;
}

/**
 * Loading state
 */
export interface LoadingState {
  readonly isLoading: boolean;
  readonly error: Error | null;
}

/**
 * Status badge variants
 */
export type BadgeVariant =
  | 'success'
  | 'error'
  | 'warning'
  | 'info'
  | 'neutral';

/**
 * Button variants and sizes
 */
export type ButtonVariant =
  | 'primary'
  | 'secondary'
  | 'danger'
  | 'ghost'
  | 'outline';

export type ButtonSize =
  | 'sm'
  | 'md'
  | 'lg';

/**
 * Table column configuration
 */
export interface TableColumn<T> {
  readonly key: keyof T | string;
  readonly label: string;
  readonly sortable?: boolean;
  readonly width?: string;
  readonly align?: 'left' | 'center' | 'right';
  readonly render?: (value: unknown, item: T) => React.ReactNode;
}

/**
 * Form field validation
 */
export interface FieldValidation {
  readonly required?: boolean;
  readonly minLength?: number;
  readonly maxLength?: number;
  readonly pattern?: RegExp;
  readonly custom?: (value: unknown) => string | null;
}

// ============================================================================
// UTILITY TYPES
// ============================================================================

/**
 * Make all properties optional recursively
 */
export type DeepPartial<T> = {
  [P in keyof T]?: T[P] extends object ? DeepPartial<T[P]> : T[P];
};

/**
 * Extract array element type
 */
export type ArrayElement<T> = T extends readonly (infer U)[] ? U : never;

/**
 * Branded type for IDs to prevent mixing different ID types
 */
export type BrandedId<T extends string> = string & { readonly __brand: T };

export type ScanId = BrandedId<'Scan'>;
export type HostId = BrandedId<'Host'>;
export type ProfileId = BrandedId<'Profile'>;
export type JobId = BrandedId<'Job'>;

/**
 * Strict object keys type
 */
export type StrictOmit<T, K extends keyof T> = Omit<T, K>;

/**
 * Non-empty array type
 */
export type NonEmptyArray<T> = [T, ...T[]];

/**
 * Async function return type
 */
export type AsyncReturnType<T extends (...args: unknown[]) => Promise<unknown>> =
  T extends (...args: unknown[]) => Promise<infer R> ? R : never;

// ============================================================================
// CONFIGURATION TYPES
// ============================================================================

/**
 * Application configuration
 */
export interface AppConfig {
  readonly api: {
    readonly baseUrl: string;
    readonly timeout: number;
    readonly retries: number;
  };
  readonly websocket: {
    readonly url: string;
    readonly reconnectInterval: number;
    readonly maxReconnectAttempts: number;
    readonly heartbeatInterval: number;
  };
  readonly ui: {
    readonly theme: 'light' | 'dark';
    readonly refreshInterval: number;
    readonly pageSize: number;
  };
  readonly features: {
    readonly realTimeUpdates: boolean;
    readonly notifications: boolean;
    readonly exportData: boolean;
  };
}

/**
 * Query options for API calls
 */
export interface QueryOptions {
  readonly enabled?: boolean;
  readonly refetchInterval?: number;
  readonly staleTime?: number;
  readonly cacheTime?: number;
  readonly retry?: number | boolean;
}

/**
 * Pagination parameters
 */
export interface PaginationParams {
  readonly page: number;
  readonly pageSize: number;
  readonly sortBy?: string;
  readonly sortOrder?: 'asc' | 'desc';
}

// ============================================================================
// TYPE GUARDS
// ============================================================================

/**
 * Type guard for checking if value is not null or undefined
 */
export const isDefined = <T>(value: T | null | undefined): value is T => {
  return value !== null && value !== undefined;
};

/**
 * Type guard for checking if value is a string
 */
export const isString = (value: unknown): value is string => {
  return typeof value === 'string';
};

/**
 * Type guard for checking if value is a number
 */
export const isNumber = (value: unknown): value is number => {
  return typeof value === 'number' && !isNaN(value);
};

/**
 * Type guard for API errors
 */
export const isApiError = (error: unknown): error is ApiError => {
  return (
    typeof error === 'object' &&
    error !== null &&
    'code' in error &&
    'message' in error &&
    'timestamp' in error
  );
};
