# Scanorama Frontend Development Plan

## Overview

This document outlines the comprehensive development plan for extending the Scanorama frontend in a structured, maintainable, and scalable way. It covers architectural patterns, development guidelines, and future roadmap.

## Current Architecture

### Technology Stack

- **Framework**: React 18 with TypeScript
- **Build Tool**: Vite for fast development and optimized builds
- **Styling**: Tailwind CSS with custom design system
- **State Management**: React Query for server state, React hooks for local state
- **Real-time**: WebSocket integration with custom service
- **API Client**: Axios with comprehensive TypeScript definitions
- **Development**: ESLint, Prettier, comprehensive linting setup

### Project Structure

```
frontend/src/
├── components/          # Reusable UI components
│   ├── dashboard/      # Dashboard-specific components
│   ├── layout/         # Layout components (Header, Sidebar, Footer)
│   └── ui/             # Generic UI components (Button, Modal, etc.)
├── hooks/              # Custom React hooks
├── pages/              # Page-level components
├── services/           # External service integrations
├── types/              # TypeScript type definitions
├── utils/              # Utility functions and helpers
└── contexts/           # React contexts (planned)
```

## Development Principles

### 1. Component Design Principles

#### Composition Over Inheritance
```typescript
// ✅ Good: Composable components
const ScanCard = ({ scan, actions, children }) => (
  <Card>
    <ScanHeader scan={scan} />
    <ScanContent scan={scan} />
    {children}
    <ScanActions>{actions}</ScanActions>
  </Card>
);

// Usage
<ScanCard scan={scan} actions={<StartButton />}>
  <ScanProgress progress={scan.progress} />
</ScanCard>
```

#### Single Responsibility Principle
```typescript
// ✅ Good: Each component has one responsibility
const ScanProgressBar = ({ progress, status }) => { /* ... */ };
const ScanStatusBadge = ({ status }) => { /* ... */ };
const ScanMetadata = ({ scan }) => { /* ... */ };

// ❌ Bad: Component doing too much
const ScanEverything = ({ scan }) => {
  // Handles progress, status, metadata, actions, etc.
};
```

#### Props Interface Design
```typescript
// ✅ Good: Clear, typed interfaces
interface ScanListProps {
  scans: Scan[];
  loading?: boolean;
  error?: Error | null;
  onScanSelect: (scan: Scan) => void;
  onScanAction: (scanId: string, action: ScanAction) => void;
  filters?: ScanFilters;
  pagination?: PaginationInfo;
}
```

### 2. State Management Strategy

#### Server State (React Query)
```typescript
// API data fetching and caching
const useScans = (filters: ScanFilters) => {
  return useQuery({
    queryKey: ['scans', filters],
    queryFn: () => apiService.getScans(filters),
    staleTime: 30000,
    refetchInterval: 60000,
  });
};
```

#### Local Component State (useState/useReducer)
```typescript
// Form state, UI state, temporary data
const [isModalOpen, setIsModalOpen] = useState(false);
const [formData, setFormData] = useState<CreateScanForm>({});
```

#### Global Application State (Context)
```typescript
// Theme, user preferences, global UI state
const AppContext = createContext<AppContextType>();
const useAppContext = () => useContext(AppContext);
```

#### Real-time State (WebSocket + React Query)
```typescript
// Automatic cache invalidation on WebSocket events
const useRealTimeScans = () => {
  const queryClient = useQueryClient();
  
  useWebSocketEvent('scan_progress', (message) => {
    queryClient.setQueryData(['scan', message.payload.scanId], (oldData) => ({
      ...oldData,
      progress: message.payload.progress,
      status: message.payload.status,
    }));
  });
};
```

## Lessons Learned from Bug Analysis

### 1. File Path Verification

**Always verify file existence before attempting operations**

```typescript
// ❌ Bad: Assuming files exist based on diagnostics
const diagnostics = await getDiagnostics();
diagnostics.forEach(diag => {
  // Directly attempting to read without verification
  const content = await readFile(diag.filePath); // May fail!
});

// ✅ Good: Verify file existence first
const verifyAndRead = async (filePath: string) => {
  try {
    const exists = await fileExists(filePath);
    if (!exists) {
      console.warn(`File not found: ${filePath}`);
      return null;
    }
    return await readFile(filePath);
  } catch (error) {
    console.error(`Error reading ${filePath}:`, error);
    return null;
  }
};
```

**Key Principles:**
- Never assume file paths from diagnostics are current/valid
- Use `find_path` to locate files by pattern before accessing
- Implement proper error handling for file operations
- Log missing files for debugging rather than failing silently

### 2. Systematic Project Exploration

**Follow a methodical approach to understanding project structure**

```typescript
// ✅ Good: Systematic exploration workflow
const exploreProject = async () => {
  // 1. Start with root directory listing
  const rootDirs = await listDirectory('.');
  
  // 2. Identify key directories (src, components, etc.)
  const srcDirs = rootDirs.filter(dir => 
    ['src', 'components', 'pages', 'services'].includes(dir.name)
  );
  
  // 3. Map out directory structure
  const projectMap = await Promise.all(
    srcDirs.map(async dir => ({
      path: dir.path,
      contents: await listDirectory(dir.path),
    }))
  );
  
  // 4. Use findings to inform subsequent operations
  return projectMap;
};
```

**Exploration Checklist:**
- [ ] List root directories first
- [ ] Identify common patterns (src/, components/, etc.)  
- [ ] Map directory structure before making assumptions
- [ ] Use `find_path` with patterns to locate specific file types
- [ ] Document discovered structure for team reference

### 3. Diagnostic Validation

**Don't blindly trust diagnostic output - validate and cross-reference**

```typescript
// ✅ Good: Validate diagnostics against actual project state
const validateDiagnostics = async (diagnostics: Diagnostic[]) => {
  const validatedDiagnostics = [];
  
  for (const diag of diagnostics) {
    // Check if file actually exists
    const fileExists = await checkFileExists(diag.filePath);
    
    if (!fileExists) {
      console.warn(`Diagnostic references non-existent file: ${diag.filePath}`);
      // Look for similar files
      const similarFiles = await findPath(`**/*${getFileName(diag.filePath)}`);
      if (similarFiles.length > 0) {
        console.info('Similar files found:', similarFiles);
      }
      continue;
    }
    
    validatedDiagnostics.push(diag);
  }
  
  return validatedDiagnostics;
};
```

**Validation Strategies:**
- Cross-reference diagnostic file paths with actual project structure
- Use pattern matching to find relocated files
- Separate actual errors from stale/phantom diagnostics
- Prioritize errors in files that actually exist

### 4. Methodical Bug Analysis

**Approach bug fixing with a structured methodology**

```typescript
// ✅ Good: Structured bug analysis workflow
const analyzeBug = async (error: DiagnosticError) => {
  // Step 1: Understand the error type and context
  const errorType = categorizeError(error);
  
  // Step 2: Gather relevant information
  const context = await gatherContext({
    filePath: error.filePath,
    dependencies: await getDependencies(error.filePath),
    relatedFiles: await findRelatedFiles(error.filePath),
  });
  
  // Step 3: Research the specific error pattern
  const knownSolutions = await lookupErrorPattern(error.message);
  
  // Step 4: Apply fix with understanding
  const fix = await applyInformedFix(error, context, knownSolutions);
  
  // Step 5: Validate fix doesn't break other things
  await validateFix(fix, context.relatedFiles);
  
  return fix;
};
```

**Bug Analysis Best Practices:**
- Understand the root cause before applying fixes
- Research error messages and patterns thoroughly
- Consider dependencies and version compatibility issues
- Test fixes against related components
- Document solutions for future reference

### 5. Dependency Version Management

**Pay attention to breaking changes in major version updates**

```typescript
// Example: React Query v4 vs v5 breaking changes
// ❌ Bad: Using deprecated v4 syntax in v5
const useScans = () => {
  return useQuery({
    queryKey: ['scans'],
    queryFn: getScans,
    keepPreviousData: true, // ❌ Removed in v5
  });
};

// ✅ Good: Updated for v5 compatibility
const useScans = () => {
  return useQuery({
    queryKey: ['scans'],
    queryFn: getScans,
    placeholderData: keepPreviousData, // ✅ v5 syntax
  });
};
```

**Version Management Guidelines:**
- Check package.json for major version updates
- Review migration guides for breaking changes
- Test thoroughly after dependency updates
- Consider gradual migration strategies for large changes

### 6. Error Communication

**Acknowledge mistakes and learn from them openly**

```typescript
// ✅ Good: Transparent error handling
const handleAnalysisError = (error: AnalysisError) => {
  console.error('Analysis failed:', {
    error: error.message,
    context: error.context,
    timestamp: new Date().toISOString(),
    // Include relevant debugging information
    projectState: getCurrentProjectState(),
    assumptions: getInitialAssumptions(),
  });
  
  // Learn from the mistake
  updateAnalysisStrategy(error.type, error.lessons);
};
```

**Communication Principles:**
- Acknowledge errors quickly and clearly
- Explain what went wrong and why
- Share lessons learned with the team
- Update processes to prevent similar mistakes
- Focus on solutions rather than blame

## Component Architecture Guidelines
</thinking>

### 1. Component Hierarchy

#### Page Components
```typescript
// Top-level page components
const DashboardPage = () => {
  // Page-level state and data fetching
  const { data: stats } = useDashboardStats();
  const { data: scans } = useRecentScans();
  
  return (
    <PageLayout>
      <DashboardHeader stats={stats} />
      <DashboardContent scans={scans} />
    </PageLayout>
  );
};
```

#### Layout Components
```typescript
// Consistent layout and navigation
const PageLayout = ({ children, sidebar = true, header = true }) => (
  <div className="min-h-screen bg-gray-900">
    {header && <Header />}
    <div className="flex">
      {sidebar && <Sidebar />}
      <main className="flex-1">{children}</main>
    </div>
  </div>
);
```

#### Feature Components
```typescript
// Feature-specific business logic
const ScanManagement = () => {
  const { scans, createScan, startScan, stopScan } = useScanManagement();
  
  return (
    <div>
      <ScanList scans={scans} onStart={startScan} onStop={stopScan} />
      <CreateScanModal onSubmit={createScan} />
    </div>
  );
};
```

#### UI Components
```typescript
// Reusable, generic components
const Button = ({ variant, size, children, ...props }) => (
  <button 
    className={cn(buttonVariants({ variant, size }))} 
    {...props}
  >
    {children}
  </button>
);
```

### 2. Component Patterns

#### Compound Components
```typescript
const ScanCard = ({ children }) => <div className="scan-card">{children}</div>;
ScanCard.Header = ({ scan }) => <div className="scan-header">{scan.name}</div>;
ScanCard.Content = ({ scan }) => <div className="scan-content">...</div>;
ScanCard.Actions = ({ children }) => <div className="scan-actions">{children}</div>;

// Usage
<ScanCard>
  <ScanCard.Header scan={scan} />
  <ScanCard.Content scan={scan} />
  <ScanCard.Actions>
    <Button onClick={onStart}>Start</Button>
  </ScanCard.Actions>
</ScanCard>
```

#### Render Props Pattern
```typescript
const DataFetcher = ({ children, ...queryProps }) => {
  const query = useQuery(queryProps);
  return children(query);
};

// Usage
<DataFetcher queryKey="scans" queryFn={getScans}>
  {({ data, isLoading, error }) => (
    <div>
      {isLoading && <LoadingSpinner />}
      {error && <ErrorMessage error={error} />}
      {data && <ScanList scans={data} />}
    </div>
  )}
</DataFetcher>
```

#### Custom Hooks Pattern
```typescript
// Encapsulate complex logic in custom hooks
const useScanActions = (scanId: string) => {
  const queryClient = useQueryClient();
  const startMutation = useStartScan();
  const stopMutation = useStopScan();
  
  const startScan = useCallback(async () => {
    await startMutation.mutateAsync(scanId);
    queryClient.invalidateQueries(['scans']);
  }, [scanId, startMutation, queryClient]);
  
  return {
    startScan,
    stopScan: () => stopMutation.mutate(scanId),
    isStarting: startMutation.isLoading,
    isStopping: stopMutation.isLoading,
  };
};
```

## API Integration Patterns

### 1. Service Layer Architecture

```typescript
// Base API service with error handling
class ApiService {
  private client: AxiosInstance;
  
  constructor() {
    this.client = axios.create({
      baseURL: '/api/v1',
      timeout: 30000,
    });
    
    this.setupInterceptors();
  }
  
  private setupInterceptors() {
    // Request interceptor for auth
    this.client.interceptors.request.use(this.addAuthHeader);
    
    // Response interceptor for error handling
    this.client.interceptors.response.use(
      (response) => response,
      this.handleError
    );
  }
}

// Feature-specific services
class ScanService extends ApiService {
  async getScans(filters: ScanFilters): Promise<PaginatedResponse<Scan>> {
    const response = await this.client.get('/scans', { params: filters });
    return response.data;
  }
  
  async createScan(data: CreateScanRequest): Promise<Scan> {
    const response = await this.client.post('/scans', data);
    return response.data;
  }
}
```

### 2. React Query Integration

```typescript
// Query key factories for consistency
const scanKeys = {
  all: ['scans'] as const,
  lists: () => [...scanKeys.all, 'list'] as const,
  list: (filters: ScanFilters) => [...scanKeys.lists(), filters] as const,
  details: () => [...scanKeys.all, 'detail'] as const,
  detail: (id: string) => [...scanKeys.details(), id] as const,
};

// Custom hooks with proper cache management
const useScans = (filters: ScanFilters) => {
  return useQuery({
    queryKey: scanKeys.list(filters),
    queryFn: () => scanService.getScans(filters),
    keepPreviousData: true,
    staleTime: 30000,
  });
};

const useCreateScan = () => {
  const queryClient = useQueryClient();
  
  return useMutation({
    mutationFn: scanService.createScan,
    onSuccess: () => {
      // Invalidate and refetch scans list
      queryClient.invalidateQueries(scanKeys.lists());
    },
    onError: (error) => {
      // Handle error (show toast, etc.)
      console.error('Failed to create scan:', error);
    },
  });
};
```

### 3. Error Handling Strategy

```typescript
// Global error boundary
class ErrorBoundary extends Component<Props, State> {
  constructor(props: Props) {
    super(props);
    this.state = { hasError: false, error: null };
  }
  
  static getDerivedStateFromError(error: Error): State {
    return { hasError: true, error };
  }
  
  componentDidCatch(error: Error, errorInfo: ErrorInfo) {
    console.error('Error caught by boundary:', error, errorInfo);
    // Send to error reporting service
  }
  
  render() {
    if (this.state.hasError) {
      return <ErrorFallback error={this.state.error} />;
    }
    
    return this.props.children;
  }
}

// API error handling
const useApiErrorHandler = () => {
  const showToast = useToast();
  
  return useCallback((error: ApiError) => {
    switch (error.status) {
      case 401:
        // Redirect to login
        window.location.href = '/login';
        break;
      case 403:
        showToast.error('Access denied');
        break;
      case 500:
        showToast.error('Server error. Please try again.');
        break;
      default:
        showToast.error(error.message || 'An error occurred');
    }
  }, [showToast]);
};
```

## Real-time Features Architecture

### 1. WebSocket Service Enhancement

```typescript
// Enhanced WebSocket service with typed events
interface WebSocketEvents {
  'scan:progress': { scanId: string; progress: number; status: ScanStatus };
  'scan:completed': { scanId: string; results: ScanResult[] };
  'host:discovered': { host: Host; scanId?: string };
  'system:status': { status: SystemStatus };
}

class TypedWebSocketService {
  private eventEmitter = new EventEmitter();
  
  on<K extends keyof WebSocketEvents>(
    event: K,
    handler: (data: WebSocketEvents[K]) => void
  ): void {
    this.eventEmitter.on(event, handler);
  }
  
  emit<K extends keyof WebSocketEvents>(
    event: K,
    data: WebSocketEvents[K]
  ): void {
    this.eventEmitter.emit(event, data);
  }
}
```

### 2. Real-time React Integration

```typescript
// Real-time data synchronization
const useRealTimeSync = () => {
  const queryClient = useQueryClient();
  
  useWebSocketEvent('scan:progress', (data) => {
    queryClient.setQueryData(
      scanKeys.detail(data.scanId),
      (oldScan: Scan) => ({
        ...oldScan,
        progress: data.progress,
        status: data.status,
      })
    );
  });
  
  useWebSocketEvent('host:discovered', (data) => {
    queryClient.setQueryData(
      hostKeys.lists(),
      (oldHosts: Host[]) => [...oldHosts, data.host]
    );
    
    // Show notification
    showNotification({
      title: 'New Host Discovered',
      message: `Found ${data.host.hostname} (${data.host.ip})`,
      type: 'success',
    });
  });
};
```

## Testing Strategy

### 1. Component Testing

```typescript
import { render, screen, userEvent } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from 'react-query';
import { ScanCard } from './ScanCard';

const createTestQueryClient = () => new QueryClient({
  defaultOptions: {
    queries: { retry: false },
    mutations: { retry: false },
  },
});

const TestWrapper = ({ children }: { children: React.ReactNode }) => {
  const queryClient = createTestQueryClient();
  return (
    <QueryClientProvider client={queryClient}>
      {children}
    </QueryClientProvider>
  );
};

describe('ScanCard', () => {
  const mockScan: Scan = {
    id: '1',
    name: 'Test Scan',
    status: 'running',
    progress: 50,
    // ... other properties
  };
  
  it('displays scan information correctly', () => {
    render(<ScanCard scan={mockScan} />, { wrapper: TestWrapper });
    
    expect(screen.getByText('Test Scan')).toBeInTheDocument();
    expect(screen.getByText('50%')).toBeInTheDocument();
  });
  
  it('handles start scan action', async () => {
    const onStart = jest.fn();
    render(<ScanCard scan={mockScan} onStart={onStart} />, { wrapper: TestWrapper });
    
    await userEvent.click(screen.getByRole('button', { name: /start/i }));
    expect(onStart).toHaveBeenCalledWith(mockScan.id);
  });
});
```

### 2. Hook Testing

```typescript
import { renderHook, waitFor } from '@testing-library/react';
import { useScanActions } from './useScanActions';

jest.mock('../services/api');

describe('useScanActions', () => {
  it('starts scan successfully', async () => {
    const { result } = renderHook(() => useScanActions('scan-1'), {
      wrapper: TestWrapper,
    });
    
    await result.current.startScan();
    
    await waitFor(() => {
      expect(result.current.isStarting).toBe(false);
    });
  });
});
```

### 3. Integration Testing

```typescript
// Test WebSocket integration
describe('Real-time updates', () => {
  it('updates scan progress from WebSocket', async () => {
    const { result } = renderHook(() => useScanProgress('scan-1'), {
      wrapper: TestWrapper,
    });
    
    // Simulate WebSocket message
    act(() => {
      webSocketService.emit('scan:progress', {
        scanId: 'scan-1',
        progress: 75,
        status: 'running',
      });
    });
    
    await waitFor(() => {
      expect(result.current.progress).toBe(75);
    });
  });
});
```

## Performance Optimization Plan

### 1. Code Splitting

```typescript
// Route-based splitting
const Dashboard = lazy(() => import('./pages/Dashboard'));
const Scans = lazy(() => import('./pages/Scans'));
const Hosts = lazy(() => import('./pages/Hosts'));

const App = () => (
  <Router>
    <Suspense fallback={<LoadingSpinner />}>
      <Routes>
        <Route path="/" element={<Dashboard />} />
        <Route path="/scans" element={<Scans />} />
        <Route path="/hosts" element={<Hosts />} />
      </Routes>
    </Suspense>
  </Router>
);
```

### 2. Component Optimization

```typescript
// Memoization for expensive components
const ScanList = memo(({ scans, onSelect }: ScanListProps) => {
  const memoizedScans = useMemo(
    () => scans.filter(scan => scan.status !== 'deleted'),
    [scans]
  );
  
  return (
    <div>
      {memoizedScans.map(scan => (
        <ScanCard key={scan.id} scan={scan} onSelect={onSelect} />
      ))}
    </div>
  );
});

// Virtualization for large lists
import { FixedSizeList as List } from 'react-window';

const VirtualizedScanList = ({ scans }: { scans: Scan[] }) => (
  <List
    height={600}
    itemCount={scans.length}
    itemSize={120}
    itemData={scans}
  >
    {({ index, style, data }) => (
      <div style={style}>
        <ScanCard scan={data[index]} />
      </div>
    )}
  </List>
);
```

### 3. Data Fetching Optimization

```typescript
// Prefetching for better UX
const usePrefetchScanDetails = () => {
  const queryClient = useQueryClient();
  
  return useCallback((scanId: string) => {
    queryClient.prefetchQuery({
      queryKey: scanKeys.detail(scanId),
      queryFn: () => scanService.getScan(scanId),
      staleTime: 60000,
    });
  }, [queryClient]);
};

// Background refetching
const useBackgroundSync = () => {
  useQuery({
    queryKey: ['background-sync'],
    queryFn: () => syncService.getUpdates(),
    refetchInterval: 30000,
    refetchIntervalInBackground: true,
  });
};
```

## Accessibility Guidelines

### 1. Component Accessibility

```typescript
// Accessible button component
const Button = forwardRef<HTMLButtonElement, ButtonProps>(
  ({ children, disabled, loading, ...props }, ref) => (
    <button
      ref={ref}
      disabled={disabled || loading}
      aria-disabled={disabled || loading}
      aria-describedby={loading ? 'loading-text' : undefined}
      {...props}
    >
      {loading && <Spinner aria-hidden="true" />}
      {children}
      {loading && (
        <span id="loading-text" className="sr-only">
          Loading...
        </span>
      )}
    </button>
  )
);
```

### 2. Form Accessibility

```typescript
const ScanForm = () => {
  const [errors, setErrors] = useState<Record<string, string>>({});
  
  return (
    <form role="form" aria-labelledby="scan-form-title">
      <h2 id="scan-form-title">Create New Scan</h2>
      
      <div className="form-group">
        <label htmlFor="scan-name">Scan Name *</label>
        <input
          id="scan-name"
          type="text"
          required
          aria-invalid={!!errors.name}
          aria-describedby={errors.name ? 'name-error' : undefined}
        />
        {errors.name && (
          <div id="name-error" role="alert" className="error">
            {errors.name}
          </div>
        )}
      </div>
    </form>
  );
};
```

### 3. Navigation Accessibility

```typescript
const Sidebar = () => (
  <nav role="navigation" aria-label="Main navigation">
    <ul role="list">
      {navigationItems.map(item => (
        <li key={item.id} role="listitem">
          <NavLink
            to={item.href}
            className={({ isActive }) =>
              cn('nav-link', { 'nav-link--active': isActive })
            }
            aria-current={item.active ? 'page' : undefined}
          >
            <item.icon aria-hidden="true" />
            {item.label}
          </NavLink>
        </li>
      ))}
    </ul>
  </nav>
);
```

## Future Development Roadmap

### Phase 1: Foundation Enhancement (Completed)
- ✅ TypeScript integration and type safety
- ✅ ESLint and Prettier setup
- ✅ WebSocket real-time features
- ✅ React Query data management
- ✅ Component architecture foundation

### Phase 2: Core Features (Next 4-6 weeks)

#### Advanced Scanning Interface
```typescript
// Multi-step scan creation wizard
const ScanWizard = () => {
  const [currentStep, setCurrentStep] = useState(0);
  const [scanConfig, setScanConfig] = useState<ScanConfig>();
  
  const steps = [
    { component: TargetSelection, title: 'Select Targets' },
    { component: ProfileConfiguration, title: 'Configure Profile' },
    { component: ScheduleSetup, title: 'Schedule Scan' },
    { component: ReviewAndSubmit, title: 'Review & Submit' },
  ];
  
  return (
    <WizardContainer>
      <WizardProgress steps={steps} currentStep={currentStep} />
      <StepComponent
        step={steps[currentStep]}
        data={scanConfig}
        onUpdate={setScanConfig}
        onNext={() => setCurrentStep(prev => prev + 1)}
        onPrevious={() => setCurrentStep(prev => prev - 1)}
      />
    </WizardContainer>
  );
};
```

#### Enhanced Data Visualization
```typescript
// Interactive scan results visualization
const ScanResultsVisualization = ({ scan }: { scan: Scan }) => {
  const [viewMode, setViewMode] = useState<'network' | 'timeline' | 'table'>('network');
  
  return (
    <div>
      <ViewModeSelector value={viewMode} onChange={setViewMode} />
      {viewMode === 'network' && <NetworkTopology scan={scan} />}
      {viewMode === 'timeline' && <ScanTimeline scan={scan} />}
      {viewMode === 'table' && <ResultsTable scan={scan} />}
    </div>
  );
};
```

#### Advanced Filtering and Search
```typescript
// Sophisticated filtering system
const useAdvancedFiltering = <T,>(data: T[], schema: FilterSchema<T>) => {
  const [filters, setFilters] = useState<FilterState>({});
  
  const filteredData = useMemo(() => {
    return data.filter(item => 
      Object.entries(filters).every(([key, value]) => 
        schema[key].matcher(item[key], value)
      )
    );
  }, [data, filters, schema]);
  
  return { filteredData, filters, setFilters };
};
```

### Phase 3: Advanced Features (6-12 weeks)

#### Dashboard Customization
```typescript
// Drag-and-drop dashboard builder
const CustomizableDashboard = () => {
  const [layout, setLayout] = useState<DashboardLayout>();
  const [availableWidgets, setAvailableWidgets] = useState<Widget[]>();
  
  return (
    <DragDropContext onDragEnd={handleDragEnd}>
      <div className="dashboard-editor">
        <WidgetPalette widgets={availableWidgets} />
        <DashboardCanvas layout={layout} onLayoutChange={setLayout} />
        <WidgetProperties selectedWidget={selectedWidget} />
      </div>
    </DragDropContext>
  );
};
```

#### Notification System
```typescript
// Real-time notification center
const NotificationCenter = () => {
  const { notifications, markAsRead, dismiss } = useNotifications();
  const [filter, setFilter] = useState<NotificationFilter>('all');
  
  return (
    <div className="notification-center">
      <NotificationFilters value={filter} onChange={setFilter} />
      <NotificationList
        notifications={notifications}
        onMarkAsRead={markAsRead}
        onDismiss={dismiss}
      />
    </div>
  );
};
```

#### Export and Reporting
```typescript
// Advanced reporting system
const ReportGenerator = () => {
  const [reportType, setReportType] = useState<ReportType>();
  const [options, setOptions] = useState<ReportOptions>();
  
  const generateReport = useCallback(async () => {
    const report = await reportService.generate(reportType, options);
    return report;
  }, [reportType, options]);
  
  return (
    <div className="report-generator">
      <ReportTypeSelector value={reportType} onChange={setReportType} />
      <ReportOptionsForm value={options} onChange={setOptions} />
      <ReportPreview reportType={reportType} options={options} />
      <ExportControls onGenerate={generateReport} />
    </div>
  );
};
```

### Phase 4: Enterprise Features (12+ weeks)

#### Multi-tenancy Support
```typescript
// Tenant-aware components
const useTenantContext = () => {
  const context = useContext(TenantContext);
  if (!context) throw new Error('Must be used within TenantProvider');
  return context;
};

const TenantAwareComponent = () => {
  const { currentTenant, switchTenant } = useTenantContext();
  const { data } = useScans({ tenantId: currentTenant.id });
  
  return (
    <div>
      <TenantSelector current={currentTenant} onSwitch={switchTenant} />
      <ScanList scans={data} />
    </div>
  );
};
```

#### Advanced Security Features
```typescript
// Role-based access control
const usePermissions = () => {
  const { user } = useAuth();
  
  const hasPermission = useCallback(
    (permission: Permission) => user.permissions.includes(permission),
    [user.permissions]
  );
  
  return { hasPermission };
};

const ProtectedComponent = ({ permission, children, fallback }: ProtectedProps) => {
  const { hasPermission } = usePermissions();
  
  if (!hasPermission(permission)) {
    return fallback || <AccessDenied />;
  }
  
  return <>{children}</>;
};
```

#### Audit Trail
```typescript
// Activity logging and audit trail
const useAuditLogger = () => {
  const logActivity = useCallback(
    (activity: AuditActivity) => {
      auditService.log({
        ...activity,
        timestamp: new Date().toISOString(),
        userId: getCurrentUser().id,
        sessionId: getSessionId(),
      });
    },
    []
  );
  
  return { logActivity };
};
```

## Development Guidelines

### 1. Code Review Checklist

- [ ] TypeScript types are comprehensive and accurate
- [ ] Components follow single responsibility principle
- [ ] Error handling is implemented properly
- [ ] Accessibility requirements are met
- [ ] Performance considerations are addressed
- [ ] Tests are written for new functionality
- [ ] Documentation is updated
- [ ] ESLint and Prettier rules are followed

### 2. Performance Requirements

- [ ] Initial page load < 2 seconds
- [ ] Component rendering < 100ms
- [ ] Real-time updates < 500ms latency
- [ ] Memory usage stays under 50MB for typical usage
- [ ] Bundle size increases < 10% per feature

### 3. Accessibility Requirements

- [ ] WCAG 2.1 AA compliance
- [ ] Keyboard navigation support
- [ ] Screen reader compatibility
- [ ] High contrast mode support
- [ ] Proper ARIA labels and roles

### 4. Testing Requirements

- [ ] Unit test coverage > 80%
- [ ] Integration tests for critical paths
- [ ] E2E tests for user workflows
- [ ] Visual regression tests for UI components
- [ ] Performance tests for heavy operations

## Conclusion

This development plan provides a structured approach to extending the Scanorama frontend while maintaining code quality, performance, and accessibility standards. The phased approach ensures steady progress while allowing for feedback and iteration.

Key success factors:
1. Consistent architecture patterns
2. Comprehensive testing strategy
3. Performance monitoring and optimization
4. Accessibility-first design
5. Proper error handling and user experience
6. Maintainable and scalable code structure

Regular reviews and updates to this plan will ensure the frontend continues to evolve effectively to meet user needs and technical requirements.