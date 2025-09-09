# Scanorama JavaScript Client Documentation

## Overview

The Scanorama JavaScript client provides a comprehensive TypeScript/React frontend with real-time WebSocket integration, robust API client, and advanced development tooling. This document covers the enhanced client implementation, linting infrastructure, and development workflow.

## Architecture

### Core Components

1. **API Service** (`src/services/api.ts`) - REST API client with Axios
2. **WebSocket Service** (`src/services/websocket.ts`) - Real-time communication
3. **React Hooks** (`src/hooks/useApi.ts`) - Custom hooks for data fetching
4. **TypeScript Types** (`src/types/index.ts`) - Comprehensive type definitions
5. **ESLint Configuration** - Code quality and consistency enforcement

## API Integration

### REST API Client

The `apiService` provides a complete client for the Scanorama REST API:

```typescript
import { apiService } from './services/api';

// Dashboard and system
const stats = await apiService.getDashboardStats();
const status = await apiService.getSystemStatus();
const health = await apiService.healthCheck();

// Scans management
const scans = await apiService.getScans(page, limit);
const scan = await apiService.getScan(scanId);
const newScan = await apiService.createScan(scanData);
await apiService.startScan(scanId);
await apiService.stopScan(scanId);
await apiService.deleteScan(scanId);

// Hosts management
const hosts = await apiService.getHosts(page, limit);
const host = await apiService.getHost(hostId);
const searchResults = await apiService.searchHosts(query);

// Profiles management
const profiles = await apiService.getProfiles();
const profile = await apiService.getProfile(profileId);
const newProfile = await apiService.createProfile(profileData);
await apiService.updateProfile(profileId, updateData);
await apiService.deleteProfile(profileId);

// Discovery jobs
const jobs = await apiService.getDiscoveryJobs(page, limit);
const job = await apiService.getDiscoveryJob(jobId);
const newJob = await apiService.createDiscoveryJob(jobData);
await apiService.startDiscoveryJob(jobId);
await apiService.stopDiscoveryJob(jobId);
```

### Features

- **Automatic authentication** with Bearer tokens
- **Request/response interceptors** for error handling
- **TypeScript support** with full type safety
- **Timeout and retry handling**
- **Automatic token refresh** on 401 responses

## WebSocket Integration

### Real-time Communication

The WebSocket service provides live updates for scan progress, host discovery, and system status:

```typescript
import { webSocketService } from './services/websocket';

// Connect to WebSocket server
await webSocketService.connect();

// Listen for events
webSocketService.on('scan_progress', (message) => {
  console.log(`Scan ${message.payload.scanId}: ${message.payload.progress}%`);
});

webSocketService.on('host_discovered', (message) => {
  console.log('New host:', message.payload.host);
});

webSocketService.on('system_status', (message) => {
  console.log('System status:', message.payload.status);
});

// Convenience methods
webSocketService.subscribeScanProgress(scanId, (progress, status) => {
  // Handle progress updates for specific scan
});

webSocketService.subscribeHostDiscovery((host, scanId) => {
  // Handle new host discoveries
});
```

### Features

- **Automatic reconnection** with exponential backoff
- **Connection status monitoring**
- **Heartbeat/ping mechanism**
- **Event-based architecture**
- **Type-safe message handling**
- **Error recovery and logging**

## React Query Integration

### Custom Hooks

We provide comprehensive React hooks for data fetching and mutations:

#### Data Fetching Hooks

```typescript
import {
  useDashboardStats,
  useSystemStatus,
  useScans,
  useScan,
  useHosts,
  useHost,
  useProfiles,
  useProfile,
  useDiscoveryJobs,
  useDiscoveryJob
} from './hooks/useApi';

function DashboardComponent() {
  const { data: stats, isLoading, error } = useDashboardStats();
  const { data: scans } = useScans(1, 10, 'running');
  const { data: hosts } = useHosts(1, 10, { status: 'up' });
  
  if (isLoading) return <div>Loading...</div>;
  if (error) return <div>Error: {error.message}</div>;
  
  return (
    <div>
      <h1>Dashboard</h1>
      <p>Running scans: {stats?.scans.running}</p>
      <p>Active hosts: {stats?.hosts.up}</p>
    </div>
  );
}
```

#### Mutation Hooks

```typescript
import {
  useCreateScan,
  useStartScan,
  useStopScan,
  useDeleteScan,
  useCreateProfile
} from './hooks/useApi';

function ScanManagement() {
  const createScan = useCreateScan();
  const startScan = useStartScan();
  const stopScan = useStopScan();
  
  const handleCreateScan = async (scanData) => {
    try {
      const newScan = await createScan.mutateAsync(scanData);
      console.log('Scan created:', newScan.id);
    } catch (error) {
      console.error('Failed to create scan:', error);
    }
  };
  
  return (
    <div>
      <button 
        onClick={() => handleCreateScan(scanData)}
        disabled={createScan.isLoading}
      >
        {createScan.isLoading ? 'Creating...' : 'Create Scan'}
      </button>
    </div>
  );
}
```

### WebSocket React Hooks

```typescript
import {
  useWebSocket,
  useScanProgress,
  useHostDiscovery,
  useScanStatus,
  useRealTimeUpdates
} from './hooks/useApi';

function ScanMonitor({ scanId }) {
  const { isConnected } = useWebSocket();
  const { progress, status, message } = useScanProgress(scanId);
  const { discoveredHosts, clearDiscoveredHosts } = useHostDiscovery();
  
  return (
    <div>
      <div>WebSocket: {isConnected ? '🟢 Connected' : '🔴 Disconnected'}</div>
      <div>Scan Progress: {progress}% ({status})</div>
      <div>Discovered Hosts: {discoveredHosts.length}</div>
      {message && <div>Message: {message}</div>}
    </div>
  );
}

function App() {
  const { isConnected, discoveredHosts } = useRealTimeUpdates();
  
  return (
    <div>
      <Header wsConnected={isConnected} />
      <HostsList newHosts={discoveredHosts} />
    </div>
  );
}
```

## TypeScript Types

### Core Domain Types

```typescript
// Scan types
interface Scan {
  id: string;
  name?: string;
  target: string;
  targets: string[];
  profile: ScanProfile;
  status: ScanStatus;
  progress: number;
  startTime: string;
  endTime?: string;
  duration?: number;
  hostsFound: number;
  portsScanned: number;
  results: ScanResult[];
  errors: string[];
  metadata: ScanMetadata;
}

// Host types
interface Host {
  id: string;
  hostname: string;
  ip: string;
  mac?: string;
  openPorts: number;
  totalPorts?: number;
  os?: string;
  osVersion?: string;
  lastSeen: string;
  status: HostStatus;
  services: Service[];
  vulnerability?: VulnerabilityInfo;
  tags: string[];
  metadata: Record<string, any>;
}

// API Request types
interface CreateScanRequest {
  name: string;
  description?: string;
  profile_id: string;
  targets: string[];
  scan_options?: Record<string, any>;
}

interface CreateDiscoveryJobRequest {
  name: string;
  network: string;
  method: 'tcp' | 'udp' | 'icmp';
}

interface CreateProfileRequest {
  name: string;
  description?: string;
  scan_type: string;
  ports?: string;
  options?: Record<string, any>;
}

// WebSocket message types
interface ScanProgressMessage extends WebSocketMessage {
  type: 'scan_progress';
  payload: {
    scanId: string;
    progress: number;
    status: ScanStatus;
    message?: string;
  };
}

interface HostDiscoveredMessage extends WebSocketMessage {
  type: 'host_discovered';
  payload: {
    host: Host;
    scanId?: string;
  };
}
```

### API Response Types

```typescript
interface ListResponse<T> {
  items: T[];
  total: number;
  page: number;
  pageSize: number;
  totalPages: number;
  hasNext: boolean;
  hasPrev: boolean;
}

interface DashboardStats {
  scans: {
    running: number;
    completed: number;
    failed: number;
    total: number;
  };
  hosts: {
    up: number;
    down: number;
    total: number;
    discovered_today: number;
  };
  performance: {
    avg_scan_duration: string;
    scans_per_hour: number;
    success_rate: number;
  };
  system: {
    uptime: number;
    version: string;
    last_update: string;
  };
}

interface SystemStatus {
  status: 'healthy' | 'warning' | 'error';
  uptime: number;
  version: string;
  database: DatabaseStatus;
  services: ServiceStatus[];
  resources: ResourceUsage;
  lastUpdate: string;
}
```

## Code Quality & Linting

### ESLint Configuration

Our ESLint setup enforces code quality and consistency:

#### Key Rules

- **TypeScript**: Strict type checking, no unused variables, prefer const assertions
- **React**: Hooks rules, JSX best practices, no prop-types required
- **Code Style**: Single quotes, semicolons, trailing commas
- **Accessibility**: Basic a11y rules for interactive elements
- **Import Organization**: Consistent import ordering and grouping

#### Configuration Files

- **`.eslintrc.json`** - Main ESLint configuration
- **`.eslintignore`** - Files to exclude from linting
- **`.prettierrc.json`** - Prettier formatting rules
- **`.prettierignore`** - Files to exclude from formatting

#### Development vs CI

- **Development**: Warnings allowed for faster iteration
- **CI**: Strict mode with no warnings for production quality

### Available Scripts

```bash
# Frontend linting
npm run lint              # Lint with warnings allowed
npm run lint:ci           # Strict CI linting (no warnings)
npm run lint:fix          # Auto-fix issues
npm run format            # Format with Prettier
npm run type-check        # TypeScript type checking

# Makefile targets (from project root)
make lint                 # Lint both backend and frontend
make lint-frontend        # Lint frontend (dev mode)
make lint-frontend-ci     # Lint frontend (CI mode)
make format-frontend      # Format and fix frontend
make lint-backend         # Lint Go backend only
make format-backend       # Format Go backend only
```

### Linting Results

Current status: **26 warnings, 0 errors**

- Console statements in development code (expected)
- Some `any` types in generic utility functions
- React hooks dependency warnings for advanced patterns

## Development Workflow

### Setup

```bash
# Install dependencies
cd frontend && npm install

# Start development server
npm run dev
# or from project root
make dev-frontend

# Run full development stack
make dev-server  # Database + API + Frontend
```

### Quality Checks

```bash
# Run all quality checks
make lint                 # Lint everything
make format               # Format everything

# Frontend-specific
make lint-frontend        # Development linting
make format-frontend      # Format and auto-fix
make lint-frontend-ci     # CI-ready linting

# Type checking
cd frontend && npm run type-check
```

### Build and Deploy

```bash
# Build for production
cd frontend && npm run build

# Preview production build
npm run preview

# Type check before build
npm run type-check
```

### Code Style Guidelines

#### TypeScript

```typescript
// ✅ Good: Explicit types, meaningful names
interface ScanCreateRequest {
  name: string;
  targets: string[];
  profileId: string;
}

const createScan = async (request: ScanCreateRequest): Promise<Scan> => {
  return apiService.createScan(request);
};

// ❌ Bad: Any types, unclear names
const doScan = async (data: any): Promise<any> => {
  return apiService.createScan(data);
};
```

#### React Components

```typescript
// ✅ Good: Typed props, clear structure
interface ScanListProps {
  scans: Scan[];
  onScanSelect: (scan: Scan) => void;
  loading?: boolean;
}

const ScanList: React.FC<ScanListProps> = ({ scans, onScanSelect, loading = false }) => {
  if (loading) {
    return <div className="loading">Loading scans...</div>;
  }

  return (
    <div className="scan-list">
      {scans.map((scan) => (
        <ScanCard 
          key={scan.id} 
          scan={scan} 
          onClick={() => onScanSelect(scan)} 
        />
      ))}
    </div>
  );
};

// ❌ Bad: No types, inline styles, unclear structure
const ScanList = ({ scans, onScanSelect }) => {
  return (
    <div style={{ padding: '10px' }}>
      {scans?.map((scan, index) => (
        <div key={index} onClick={() => onScanSelect(scan)}>
          {scan.name}
        </div>
      ))}
    </div>
  );
};
```

#### Hooks Usage

```typescript
// ✅ Good: Proper error handling, loading states
const useScanManagement = (scanId: string) => {
  const { data: scan, isLoading, error } = useScan(scanId);
  const startMutation = useStartScan();
  const stopMutation = useStopScan();

  const startScan = useCallback(async () => {
    try {
      await startMutation.mutateAsync(scanId);
    } catch (error) {
      console.error('Failed to start scan:', error);
      throw error;
    }
  }, [scanId, startMutation]);

  return {
    scan,
    isLoading,
    error,
    startScan,
    isStarting: startMutation.isLoading,
    canStart: scan?.status === 'queued' && !startMutation.isLoading,
  };
};

// ❌ Bad: No error handling, unclear return
const useScanManagement = (scanId) => {
  const scan = useScan(scanId);
  const start = useStartScan();
  
  return { scan, start };
};
```

## Testing Strategy

### Unit Testing

```typescript
import { render, screen, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from 'react-query';
import userEvent from '@testing-library/user-event';
import { ScanList } from './ScanList';

describe('ScanList', () => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });

  const wrapper = ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={queryClient}>
      {children}
    </QueryClientProvider>
  );

  it('displays loading state', () => {
    render(<ScanList scans={[]} loading />, { wrapper });
    expect(screen.getByText('Loading scans...')).toBeInTheDocument();
  });

  it('handles scan selection', async () => {
    const mockOnSelect = jest.fn();
    const scans = [{ id: '1', name: 'Test Scan', status: 'completed' }];
    
    render(<ScanList scans={scans} onScanSelect={mockOnSelect} />, { wrapper });
    
    await userEvent.click(screen.getByText('Test Scan'));
    expect(mockOnSelect).toHaveBeenCalledWith(scans[0]);
  });
});
```

### Integration Testing

```typescript
import { renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from 'react-query';
import { useScanProgress } from '../hooks/useApi';
import { webSocketService } from '../services/websocket';

jest.mock('../services/websocket');

describe('useScanProgress', () => {
  it('updates progress from WebSocket messages', async () => {
    const { result } = renderHook(() => useScanProgress('scan-123'), {
      wrapper: ({ children }) => (
        <QueryClientProvider client={new QueryClient()}>
          {children}
        </QueryClientProvider>
      ),
    });

    // Simulate WebSocket message
    const mockMessage = {
      type: 'scan_progress',
      payload: { scanId: 'scan-123', progress: 50, status: 'running' },
      timestamp: new Date().toISOString(),
    };

    // Trigger the WebSocket handler
    webSocketService.emit('scan_progress', mockMessage);

    await waitFor(() => {
      expect(result.current.progress).toBe(50);
      expect(result.current.status).toBe('running');
    });
  });
});
```

## Performance Considerations

### React Query Caching

```typescript
// Optimized caching strategy
export function useScans(page = 1, limit = 10, status?: string, target?: string) {
  const queryKey = ['scans', page, limit, status, target].filter(Boolean);
  return useApi(
    queryKey,
    () => apiService.getScans(page, limit),
    {
      keepPreviousData: true,    // Smooth pagination
      staleTime: 30000,          // 30 second cache
      refetchInterval: 60000,    // Auto-refresh every minute
      refetchOnWindowFocus: false, // Prevent excessive requests
    }
  );
}
```

### WebSocket Optimization

```typescript
// Efficient event handling
const webSocketService = new WebSocketService({
  reconnectInterval: 3000,        // 3 second base interval
  maxReconnectAttempts: 5,        // Maximum retry attempts
  heartbeatInterval: 30000,       // 30 second heartbeat
  debug: process.env.NODE_ENV === 'development',
});

// Cleanup subscriptions
useEffect(() => {
  const handler = (message) => { /* handle message */ };
  webSocketService.on('scan_progress', handler);
  
  return () => {
    webSocketService.off('scan_progress', handler);
  };
}, []);
```

### Bundle Optimization

- **Tree shaking**: Only import used functions
- **Code splitting**: Lazy load components
- **Asset optimization**: Optimize images and fonts
- **Dependency analysis**: Regular audit of bundle size

## Security Considerations

### API Security

- **Token management**: Secure token storage and refresh
- **Request validation**: Client-side validation before API calls
- **Error handling**: No sensitive data in error messages
- **HTTPS enforcement**: Secure communication channels

### WebSocket Security

- **Authentication**: Token-based WebSocket authentication
- **Message validation**: Validate all incoming messages
- **Connection limits**: Prevent connection abuse
- **Error recovery**: Graceful handling of connection failures

## Troubleshooting

### Common Issues

1. **ESLint Configuration Errors**
   ```bash
   # Clear ESLint cache
   cd frontend && npx eslint --cache-location=/tmp/eslint-cache src/
   
   # Check configuration
   npx eslint --print-config src/App.tsx
   ```

2. **TypeScript Errors**
   ```bash
   # Full type check
   npm run type-check
   
   # Clear TypeScript cache
   rm -rf node_modules/.cache/typescript
   ```

3. **WebSocket Connection Issues**
   ```javascript
   // Debug WebSocket connection
   webSocketService.options.debug = true;
   
   // Check connection status
   console.log('WebSocket state:', webSocketService.readyState);
   console.log('Reconnect attempts:', webSocketService.reconnectAttemptsCount);
   ```

4. **API Connection Problems**
   ```javascript
   // Test API connectivity
   apiService.healthCheck()
     .then(() => console.log('API is reachable'))
     .catch(error => console.error('API error:', error));
   ```

### Development Tips

- Use React Developer Tools for component inspection
- Enable WebSocket debugging in browser DevTools
- Monitor network requests in DevTools Network tab
- Use React Query DevTools for cache inspection
- Check console for ESLint warnings during development

## Future Enhancements

### Planned Features

1. **Enhanced Testing**: Jest + Testing Library setup
2. **Storybook Integration**: Component documentation and testing
3. **PWA Support**: Offline capabilities and push notifications
4. **Advanced Filtering**: More sophisticated search and filter options
5. **Data Visualization**: Charts and graphs for scan results
6. **Export Functionality**: PDF and CSV export capabilities
7. **Theme Support**: Light/dark theme switching
8. **Internationalization**: Multi-language support

### Contributing

1. Follow ESLint and Prettier configurations
2. Write comprehensive TypeScript types
3. Add proper error handling and loading states
4. Test WebSocket integrations thoroughly
5. Update documentation for new features
6. Maintain backward compatibility with API changes

---

This documentation provides a comprehensive overview of the enhanced Scanorama JavaScript client. For specific implementation details, refer to the source code and inline comments.