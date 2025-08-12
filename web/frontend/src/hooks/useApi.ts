// React Hooks for API Data Fetching
// Provides React-friendly hooks with loading states, error handling, and real-time updates

import { useState, useEffect, useCallback, useRef } from "react";
import {
  apiClient,
  scanAPI,
  hostAPI,
  discoveryAPI,
  profileAPI,
  scheduleAPI,
  systemAPI,
} from "../services/api";
import logger from "../utils/logger";
import {
  ScanResponse,
  ScanRequest,
  ScanFilters,
  ScanResultsResponse,
  Host,
  HostFilters,
  DiscoveryJob,
  ScanProfile,
  Schedule,
  SystemStatus,
  HealthCheck,
  DashboardStats,
  PaginationParams,
  APIError,
  UUID,
  ScanUpdateMessage,
  DiscoveryUpdateMessage,
} from "../types/api";

// Base hook interfaces
interface UseApiState<T> {
  data: T | null;
  loading: boolean;
  error: APIError | null;
}

interface UseApiListState<T> {
  data: T[];
  loading: boolean;
  error: APIError | null;
  pagination: {
    page: number;
    pageSize: number;
    totalItems: number;
    totalPages: number;
  } | null;
}

interface UseApiMutationState {
  loading: boolean;
  error: APIError | null;
}

// Generic hooks for common patterns
export function useApiState<T>(
  initialData: T | null = null,
): [UseApiState<T>, (data: T | null, loading?: boolean, error?: APIError | null) => void] {
  const [state, setState] = useState<UseApiState<T>>({
    data: initialData,
    loading: false,
    error: null,
  });

  const updateState = useCallback(
    (data: T | null, loading = false, error: APIError | null = null) => {
      setState({ data, loading, error });
    },
    [],
  );

  return [state, updateState];
}

// System Status and Health Hooks
export function useSystemStatus(autoRefresh = true, refreshInterval = 30000) {
  const [state, updateState] = useApiState<SystemStatus>();
  const intervalRef = useRef<NodeJS.Timeout>();

  const fetchStatus = useCallback(async () => {
    updateState(null, true);
    try {
      const status = await systemAPI.getStatus();
      updateState(status, false);
    } catch (error) {
      logger.error("Failed to fetch system status", {
        context: {
          endpoint: "/api/system/status",
          method: "GET",
          component: "useSystemStatus",
        },
        error: error as Error,
      });
      updateState(null, false, error as APIError);
    }
  }, [updateState]);

  useEffect(() => {
    fetchStatus();

    if (autoRefresh) {
      intervalRef.current = setInterval(fetchStatus, refreshInterval);
    }

    return () => {
      if (intervalRef.current) {
        clearInterval(intervalRef.current);
      }
    };
  }, [fetchStatus, autoRefresh, refreshInterval]);

  return { ...state, refetch: fetchStatus };
}

export function useHealthCheck() {
  const [state, updateState] = useApiState<HealthCheck>();

  const fetchHealth = useCallback(async () => {
    updateState(null, true);
    try {
      const health = await systemAPI.getHealth();
      updateState(health, false);
    } catch (error) {
      logger.error("Failed to fetch health check", {
        context: {
          endpoint: "/api/system/health",
          method: "GET",
          component: "useHealthCheck",
        },
        error: error as Error,
      });
      updateState(null, false, error as APIError);
    }
  }, [updateState]);

  useEffect(() => {
    fetchHealth();
  }, [fetchHealth]);

  return { ...state, refetch: fetchHealth };
}

export function useDashboardStats(autoRefresh = true, refreshInterval = 10000) {
  const [state, updateState] = useApiState<DashboardStats>();
  const intervalRef = useRef<NodeJS.Timeout>();

  const fetchStats = useCallback(async () => {
    updateState(null, true);
    try {
      const stats = await systemAPI.getDashboardStats();
      updateState(stats, false);
    } catch (error) {
      logger.error("Failed to fetch dashboard stats", {
        context: {
          endpoint: "/api/system/stats",
          method: "GET",
          component: "useDashboardStats",
        },
        error: error as Error,
      });
      updateState(null, false, error as APIError);
    }
  }, [updateState]);

  useEffect(() => {
    fetchStats();

    if (autoRefresh) {
      intervalRef.current = setInterval(fetchStats, refreshInterval);
    }

    return () => {
      if (intervalRef.current) {
        clearInterval(intervalRef.current);
      }
    };
  }, [fetchStats, autoRefresh, refreshInterval]);

  return { ...state, refetch: fetchStats };
}

// Scan Hooks
export function useScans(filters?: ScanFilters, pagination?: PaginationParams) {
  const [state, setState] = useState<UseApiListState<ScanResponse>>({
    data: [],
    loading: false,
    error: null,
    pagination: null,
  });

  const fetchScans = useCallback(async () => {
    setState((prev) => ({ ...prev, loading: true, error: null }));
    try {
      const response = await scanAPI.list(filters, pagination);
      setState({
        data: response.data,
        loading: false,
        error: null,
        pagination: response.pagination
          ? {
              page: response.pagination.page,
              pageSize: response.pagination.page_size,
              totalItems: response.pagination.total_items,
              totalPages: response.pagination.total_pages,
            }
          : null,
      });
    } catch (error) {
      setState((prev) => ({
        ...prev,
        loading: false,
        error: error as APIError,
      }));
    }
  }, [filters, pagination]);

  useEffect(() => {
    fetchScans();
  }, [fetchScans]);

  return { ...state, refetch: fetchScans };
}

export function useScan(id: UUID | null) {
  const [state, updateState] = useApiState<ScanResponse>();

  const fetchScan = useCallback(async () => {
    if (!id) return;

    updateState(null, true);
    try {
      const scan = await scanAPI.get(id);
      updateState(scan, false);
    } catch (error) {
      logger.error(`Failed to fetch scan ${id}`, {
        context: {
          endpoint: `/api/scans/${id}`,
          method: "GET",
          component: "useScan",
        },
        error: error as Error,
      });
      updateState(null, false, error as APIError);
    }
  }, [id, updateState]);

  useEffect(() => {
    fetchScan();
  }, [fetchScan]);

  return { ...state, refetch: fetchScan };
}

export function useScanResults(scanId: UUID | null, pagination?: PaginationParams) {
  const [state, updateState] = useApiState<ScanResultsResponse>();

  const fetchResults = useCallback(async () => {
    if (!scanId) return;

    updateState(null, true);
    try {
      const results = await scanAPI.getResults(scanId, pagination);
      updateState(results, false);
    } catch (error) {
      logger.error(`Failed to fetch scan results for ${scanId}`, {
        context: {
          endpoint: `/api/scans/${scanId}/results`,
          method: "GET",
          component: "useScanResults",
        },
        error: error as Error,
      });
      updateState(null, false, error as APIError);
    }
  }, [scanId, pagination, updateState]);

  useEffect(() => {
    fetchResults();
  }, [fetchResults]);

  return { ...state, refetch: fetchResults };
}

export function useScanMutations() {
  const [createState, setCreateState] = useState<UseApiMutationState>({
    loading: false,
    error: null,
  });
  const [updateState, setUpdateState] = useState<UseApiMutationState>({
    loading: false,
    error: null,
  });
  const [deleteState, setDeleteState] = useState<UseApiMutationState>({
    loading: false,
    error: null,
  });
  const [controlState, setControlState] = useState<UseApiMutationState>({
    loading: false,
    error: null,
  });

  const createScan = useCallback(async (scanRequest: ScanRequest): Promise<ScanResponse | null> => {
    setCreateState({ loading: true, error: null });
    try {
      const scan = await scanAPI.create(scanRequest);
      logger.info(
        "Scan created successfully",
        {
          component: "useScanMutations",
          action: "create",
        },
        { scanName: scanRequest.name },
      );
      setCreateState({ loading: false, error: null });
      return scan;
    } catch (error) {
      logger.error("Failed to create scan", {
        context: {
          endpoint: "/api/scans",
          method: "POST",
          component: "useScanMutations",
          action: "create",
        },
        error: error as Error,
      });
      setCreateState({ loading: false, error: error as APIError });
      return null;
    }
  }, []);

  const updateScan = useCallback(
    async (id: UUID, updates: Partial<ScanRequest>): Promise<ScanResponse | null> => {
      setUpdateState({ loading: true, error: null });
      try {
        const scan = await scanAPI.update(id, updates);
        logger.info(
          "Scan updated successfully",
          {
            component: "useScanMutations",
            action: "update",
          },
          { scanId: id },
        );
        setUpdateState({ loading: false, error: null });
        return scan;
      } catch (error) {
        logger.error(`Failed to update scan ${id}`, {
          context: {
            endpoint: `/api/scans/${id}`,
            method: "PUT",
            component: "useScanMutations",
            action: "update",
          },
          error: error as Error,
        });
        setUpdateState({ loading: false, error: error as APIError });
        return null;
      }
    },
    [],
  );

  const deleteScan = useCallback(async (id: UUID): Promise<boolean> => {
    setDeleteState({ loading: true, error: null });
    try {
      await scanAPI.delete(id);
      logger.info(
        "Scan deleted successfully",
        {
          component: "useScanMutations",
          action: "delete",
        },
        { scanId: id },
      );
      setDeleteState({ loading: false, error: null });
      return true;
    } catch (error) {
      logger.error(`Failed to delete scan ${id}`, {
        context: {
          endpoint: `/api/scans/${id}`,
          method: "DELETE",
          component: "useScanMutations",
          action: "delete",
        },
        error: error as Error,
      });
      setDeleteState({ loading: false, error: error as APIError });
      return false;
    }
  }, []);

  const startScan = useCallback(async (id: UUID): Promise<ScanResponse | null> => {
    setControlState({ loading: true, error: null });
    try {
      const scan = await scanAPI.start(id);
      logger.info(
        "Scan started successfully",
        {
          component: "useScanMutations",
          action: "start",
        },
        { scanId: id },
      );
      setControlState({ loading: false, error: null });
      return scan;
    } catch (error) {
      logger.error(`Failed to start scan ${id}`, {
        context: {
          endpoint: `/api/scans/${id}/start`,
          method: "POST",
          component: "useScanMutations",
          action: "start",
        },
        error: error as Error,
      });
      setControlState({ loading: false, error: error as APIError });
      return null;
    }
  }, []);

  const stopScan = useCallback(async (id: UUID): Promise<boolean> => {
    setControlState({ loading: true, error: null });
    try {
      await scanAPI.stop(id);
      logger.info(
        "Scan stopped successfully",
        {
          component: "useScanMutations",
          action: "stop",
        },
        { scanId: id },
      );
      setControlState({ loading: false, error: null });
      return true;
    } catch (error) {
      logger.error(`Failed to stop scan ${id}`, {
        context: {
          endpoint: `/api/scans/${id}/stop`,
          method: "POST",
          component: "useScanMutations",
          action: "stop",
        },
        error: error as Error,
      });
      setControlState({ loading: false, error: error as APIError });
      return false;
    }
  }, []);

  return {
    create: { ...createState, execute: createScan },
    update: { ...updateState, execute: updateScan },
    delete: { ...deleteState, execute: deleteScan },
    start: { ...controlState, execute: startScan },
    stop: { ...controlState, execute: stopScan },
  };
}

// Host Hooks
export function useHosts(filters?: HostFilters, pagination?: PaginationParams) {
  const [state, setState] = useState<UseApiListState<Host>>({
    data: [],
    loading: false,
    error: null,
    pagination: null,
  });

  const fetchHosts = useCallback(async () => {
    setState((prev) => ({ ...prev, loading: true, error: null }));
    try {
      const response = await hostAPI.list(filters, pagination);
      setState({
        data: response.data,
        loading: false,
        error: null,
        pagination: response.pagination
          ? {
              page: response.pagination.page,
              pageSize: response.pagination.page_size,
              totalItems: response.pagination.total_items,
              totalPages: response.pagination.total_pages,
            }
          : null,
      });
    } catch (error) {
      setState((prev) => ({
        ...prev,
        loading: false,
        error: error as APIError,
      }));
    }
  }, [filters, pagination]);

  useEffect(() => {
    fetchHosts();
  }, [fetchHosts]);

  return { ...state, refetch: fetchHosts };
}

export function useHost(id: UUID | null) {
  const [state, updateState] = useApiState<Host>();

  const fetchHost = useCallback(async () => {
    if (!id) return;

    updateState(null, true);
    try {
      const host = await hostAPI.get(id);
      updateState(host, false);
    } catch (error) {
      updateState(null, false, error as APIError);
    }
  }, [id, updateState]);

  useEffect(() => {
    fetchHost();
  }, [fetchHost]);

  return { ...state, refetch: fetchHost };
}

// Discovery Hooks
export function useDiscoveryJobs(pagination?: PaginationParams) {
  const [state, setState] = useState<UseApiListState<DiscoveryJob>>({
    data: [],
    loading: false,
    error: null,
    pagination: null,
  });

  const fetchJobs = useCallback(async () => {
    setState((prev) => ({ ...prev, loading: true, error: null }));
    try {
      const response = await discoveryAPI.list(pagination);
      setState({
        data: response.data,
        loading: false,
        error: null,
        pagination: response.pagination
          ? {
              page: response.pagination.page,
              pageSize: response.pagination.page_size,
              totalItems: response.pagination.total_items,
              totalPages: response.pagination.total_pages,
            }
          : null,
      });
    } catch (error) {
      setState((prev) => ({
        ...prev,
        loading: false,
        error: error as APIError,
      }));
    }
  }, [pagination]);

  useEffect(() => {
    fetchJobs();
  }, [fetchJobs]);

  return { ...state, refetch: fetchJobs };
}

// Profile Hooks
export function useProfiles(pagination?: PaginationParams) {
  const [state, setState] = useState<UseApiListState<ScanProfile>>({
    data: [],
    loading: false,
    error: null,
    pagination: null,
  });

  const fetchProfiles = useCallback(async () => {
    setState((prev) => ({ ...prev, loading: true, error: null }));
    try {
      const response = await profileAPI.list(pagination);
      setState({
        data: response.data,
        loading: false,
        error: null,
        pagination: response.pagination
          ? {
              page: response.pagination.page,
              pageSize: response.pagination.page_size,
              totalItems: response.pagination.total_items,
              totalPages: response.pagination.total_pages,
            }
          : null,
      });
    } catch (error) {
      setState((prev) => ({
        ...prev,
        loading: false,
        error: error as APIError,
      }));
    }
  }, [pagination]);

  useEffect(() => {
    fetchProfiles();
  }, [fetchProfiles]);

  return { ...state, refetch: fetchProfiles };
}

// Schedule Hooks
export function useSchedules(pagination?: PaginationParams) {
  const [state, setState] = useState<UseApiListState<Schedule>>({
    data: [],
    loading: false,
    error: null,
    pagination: null,
  });

  const fetchSchedules = useCallback(async () => {
    setState((prev) => ({ ...prev, loading: true, error: null }));
    try {
      const response = await scheduleAPI.list(pagination);
      setState({
        data: response.data,
        loading: false,
        error: null,
        pagination: response.pagination
          ? {
              page: response.pagination.page,
              pageSize: response.pagination.page_size,
              totalItems: response.pagination.total_items,
              totalPages: response.pagination.total_pages,
            }
          : null,
      });
    } catch (error) {
      setState((prev) => ({
        ...prev,
        loading: false,
        error: error as APIError,
      }));
    }
  }, [pagination]);

  useEffect(() => {
    fetchSchedules();
  }, [fetchSchedules]);

  return { ...state, refetch: fetchSchedules };
}

// WebSocket Hooks for Real-time Updates
export function useScanUpdates(onUpdate?: (update: ScanUpdateMessage) => void) {
  const [connected, setConnected] = useState(false);
  const [lastUpdate, setLastUpdate] = useState<ScanUpdateMessage | null>(null);
  const [error, setError] = useState<string | null>(null);

  const connect = useCallback(() => {
    setError(null);
    apiClient.connectToScanUpdates(
      (update: ScanUpdateMessage) => {
        setLastUpdate(update);
        if (onUpdate) {
          onUpdate(update);
        }
      },
      (_error: Event) => {
        setError("WebSocket connection error");
        setConnected(false);
      },
      (event: CloseEvent) => {
        setConnected(false);
        if (!event.wasClean) {
          setError("WebSocket connection lost");
        }
      },
    );
    setConnected(true);
  }, [onUpdate]);

  const disconnect = useCallback(() => {
    apiClient.disconnectWebSocket("scans");
    setConnected(false);
    setError(null);
  }, []);

  useEffect(() => {
    connect();
    return () => disconnect();
  }, [connect, disconnect]);

  return {
    connected,
    lastUpdate,
    error,
    connect,
    disconnect,
  };
}

export function useDiscoveryUpdates(onUpdate?: (update: DiscoveryUpdateMessage) => void) {
  const [connected, setConnected] = useState(false);
  const [lastUpdate, setLastUpdate] = useState<DiscoveryUpdateMessage | null>(null);
  const [error, setError] = useState<string | null>(null);

  const connect = useCallback(() => {
    setError(null);
    apiClient.connectToDiscoveryUpdates(
      (update: DiscoveryUpdateMessage) => {
        setLastUpdate(update);
        if (onUpdate) {
          onUpdate(update);
        }
      },
      (_error: Event) => {
        setError("WebSocket connection error");
        setConnected(false);
      },
      (event: CloseEvent) => {
        setConnected(false);
        if (!event.wasClean) {
          setError("WebSocket connection lost");
        }
      },
    );
    setConnected(true);
  }, [onUpdate]);

  const disconnect = useCallback(() => {
    apiClient.disconnectWebSocket("discovery");
    setConnected(false);
    setError(null);
  }, []);

  useEffect(() => {
    connect();
    return () => disconnect();
  }, [connect, disconnect]);

  return {
    connected,
    lastUpdate,
    error,
    connect,
    disconnect,
  };
}

// Pagination Hook
export function usePagination(initialPage = 1, initialPageSize = 20) {
  const [page, setPage] = useState(initialPage);
  const [pageSize, setPageSize] = useState(initialPageSize);

  const paginationParams: PaginationParams = {
    page,
    page_size: pageSize,
    offset: (page - 1) * pageSize,
  };

  const nextPage = useCallback(() => setPage((prev) => prev + 1), []);
  const prevPage = useCallback(() => setPage((prev) => Math.max(1, prev - 1)), []);
  const goToPage = useCallback((newPage: number) => setPage(Math.max(1, newPage)), []);
  const changePageSize = useCallback((newSize: number) => {
    setPageSize(newSize);
    setPage(1); // Reset to first page when changing page size
  }, []);

  return {
    page,
    pageSize,
    params: paginationParams,
    nextPage,
    prevPage,
    goToPage,
    changePageSize,
    setPage,
    setPageSize,
  };
}

// Debounced search hook
export function useDebounce<T>(value: T, delay: number): T {
  const [debouncedValue, setDebouncedValue] = useState<T>(value);

  useEffect(() => {
    const handler = setTimeout(() => {
      setDebouncedValue(value);
    }, delay);

    return () => {
      clearTimeout(handler);
    };
  }, [value, delay]);

  return debouncedValue;
}

// Combined hook for scan management with real-time updates
export function useScanManagement(filters?: ScanFilters) {
  const pagination = usePagination();
  const scansQuery = useScans(filters, pagination.params);
  const mutations = useScanMutations();

  // Real-time updates
  useScanUpdates((update: ScanUpdateMessage) => {
    // Update scan in the list if it exists
    if (scansQuery.data.length > 0) {
      scansQuery.data.map((scan) => {
        if (scan.id === update.scan_id) {
          return {
            ...scan,
            status: update.status,
            progress: update.progress,
            start_time: update.start_time || scan.start_time,
            end_time: update.end_time || scan.end_time,
          };
        }
        return scan;
      });

      // Force re-render with updated data
      scansQuery.refetch();
    }
  });

  const createScan = useCallback(
    async (scanRequest: ScanRequest) => {
      const result = await mutations.create.execute(scanRequest);
      if (result) {
        // Refresh the list after successful creation
        scansQuery.refetch();
      }
      return result;
    },
    [mutations.create, scansQuery],
  );

  const startScan = useCallback(
    async (id: UUID) => {
      const result = await mutations.start.execute(id);
      if (result) {
        scansQuery.refetch();
      }
      return result;
    },
    [mutations.start, scansQuery],
  );

  const stopScan = useCallback(
    async (id: UUID) => {
      const result = await mutations.stop.execute(id);
      if (result) {
        scansQuery.refetch();
      }
      return result;
    },
    [mutations.stop, scansQuery],
  );

  const deleteScan = useCallback(
    async (id: UUID) => {
      const result = await mutations.delete.execute(id);
      if (result) {
        scansQuery.refetch();
      }
      return result;
    },
    [mutations.delete, scansQuery],
  );

  return {
    scans: scansQuery,
    pagination,
    mutations: {
      create: { ...mutations.create, execute: createScan },
      update: mutations.update,
      delete: { ...mutations.delete, execute: deleteScan },
      start: { ...mutations.start, execute: startScan },
      stop: { ...mutations.stop, execute: stopScan },
    },
  };
}

// Error handling hook
export function useErrorHandler() {
  const [errors, setErrors] = useState<APIError[]>([]);

  const addError = useCallback((error: APIError) => {
    setErrors((prev) => [...prev, { ...error, timestamp: new Date().toISOString() }]);
  }, []);

  const removeError = useCallback((index: number) => {
    setErrors((prev) => prev.filter((_, i) => i !== index));
  }, []);

  const clearErrors = useCallback(() => {
    setErrors([]);
  }, []);

  return {
    errors,
    addError,
    removeError,
    clearErrors,
    hasErrors: errors.length > 0,
  };
}

// Local storage hook for persisting UI state
export function useLocalStorage<T>(key: string, initialValue: T): [T, (value: T) => void] {
  const [storedValue, setStoredValue] = useState<T>(() => {
    try {
      const item = window.localStorage.getItem(key);
      return item ? JSON.parse(item) : initialValue;
    } catch (error) {
      console.warn(`Error reading localStorage key "${key}":`, error);
      return initialValue;
    }
  });

  const setValue = useCallback(
    (value: T) => {
      try {
        setStoredValue(value);
        window.localStorage.setItem(key, JSON.stringify(value));
      } catch (error) {
        console.warn(`Error setting localStorage key "${key}":`, error);
      }
    },
    [key],
  );

  return [storedValue, setValue];
}

// Hook for managing async operations with loading states
export function useAsyncOperation<T extends any[], R>(
  operation: (...args: T) => Promise<R>,
): [
  { loading: boolean; error: APIError | null; data: R | null },
  (...args: T) => Promise<R | null>,
] {
  const [state, setState] = useState<{
    loading: boolean;
    error: APIError | null;
    data: R | null;
  }>({
    loading: false,
    error: null,
    data: null,
  });

  const execute = useCallback(
    async (...args: T): Promise<R | null> => {
      setState({ loading: true, error: null, data: null });
      try {
        const result = await operation(...args);
        setState({ loading: false, error: null, data: result });
        return result;
      } catch (error) {
        setState({ loading: false, error: error as APIError, data: null });
        return null;
      }
    },
    [operation],
  );

  return [state, execute];
}

// Hook for managing form state and validation
export function useFormState<T extends Record<string, any>>(
  initialState: T,
  validator?: (values: T) => Record<string, string>,
) {
  const [values, setValues] = useState<T>(initialState);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [touched, setTouched] = useState<Record<string, boolean>>({});

  const setValue = useCallback((field: keyof T, value: any) => {
    setValues((prev) => ({ ...prev, [field]: value }));
    setTouched((prev) => ({ ...prev, [field]: true }));
  }, []);

  const setFieldError = useCallback((field: keyof T, error: string) => {
    setErrors((prev) => ({ ...prev, [field]: error }));
  }, []);

  const clearFieldError = useCallback((field: keyof T) => {
    setErrors((prev) => {
      const newErrors = { ...prev };
      delete newErrors[field as string];
      return newErrors;
    });
  }, []);

  const validate = useCallback(() => {
    if (validator) {
      const validationErrors = validator(values);
      setErrors(validationErrors);
      return Object.keys(validationErrors).length === 0;
    }
    return true;
  }, [values, validator]);

  const reset = useCallback(() => {
    setValues(initialState);
    setErrors({});
    setTouched({});
  }, [initialState]);

  const isValid = Object.keys(errors).length === 0;
  const isDirty = Object.keys(touched).length > 0;

  return {
    values,
    errors,
    touched,
    isValid,
    isDirty,
    setValue,
    setFieldError,
    clearFieldError,
    validate,
    reset,
  };
}
