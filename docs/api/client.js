/**
 * Scanorama API Client
 *
 * A comprehensive JavaScript client for the Scanorama API.
 * Supports all endpoints with built-in error handling, retries, and real-time monitoring.
 *
 * @example
 * const client = new ScanoramaClient('http://localhost:8080');
 * const scans = await client.scans.list();
 * const newScan = await client.scans.create({
 *   name: 'My Scan',
 *   targets: ['192.168.1.0/24'],
 *   scan_type: 'connect'
 * });
 */

class ScanoramaClient {
  constructor(baseUrl = "http://localhost:8080", options = {}) {
    this.baseUrl = baseUrl.replace(/\/$/, ""); // Remove trailing slash
    this.apiUrl = `${this.baseUrl}/api/v1`;
    this.options = {
      timeout: 30000,
      retries: 3,
      retryDelay: 1000,
      ...options,
    };

    // Initialize endpoint handlers
    this.scans = new ScanEndpoints(this);
    this.hosts = new HostEndpoints(this);
    this.profiles = new ProfileEndpoints(this);
    this.discovery = new DiscoveryEndpoints(this);
    this.schedules = new ScheduleEndpoints(this);
    this.health = new HealthEndpoints(this);
  }

  /**
   * Make a raw API call
   */
  async call(endpoint, options = {}) {
    const url = endpoint.startsWith("http")
      ? endpoint
      : `${this.apiUrl}${endpoint}`;

    const requestOptions = {
      headers: {
        "Content-Type": "application/json",
        ...options.headers,
      },
      ...options,
    };

    // Add request timeout
    const controller = new AbortController();
    const timeoutId = setTimeout(
      () => controller.abort(),
      this.options.timeout,
    );
    requestOptions.signal = controller.signal;

    try {
      const response = await fetch(url, requestOptions);
      clearTimeout(timeoutId);

      if (!response.ok) {
        const errorData = await response.json().catch(() => ({}));
        const error = new Error(
          errorData.error || `HTTP ${response.status}: ${response.statusText}`,
        );
        error.status = response.status;
        error.response = response;
        error.data = errorData;
        throw error;
      }

      // Handle no-content responses
      if (response.status === 204) {
        return null;
      }

      return await response.json();
    } catch (error) {
      clearTimeout(timeoutId);

      if (error.name === "AbortError") {
        throw new Error("Request timeout");
      }

      throw error;
    }
  }

  /**
   * Make an API call with automatic retries
   */
  async callWithRetry(endpoint, options = {}) {
    let lastError;

    for (let attempt = 1; attempt <= this.options.retries; attempt++) {
      try {
        return await this.call(endpoint, options);
      } catch (error) {
        lastError = error;

        // Don't retry on client errors (4xx) except 429 (rate limit)
        if (error.status >= 400 && error.status < 500 && error.status !== 429) {
          throw error;
        }

        // Wait before retry (exponential backoff)
        if (attempt < this.options.retries) {
          await new Promise((resolve) =>
            setTimeout(
              resolve,
              this.options.retryDelay * Math.pow(2, attempt - 1),
            ),
          );
        }
      }
    }

    throw lastError;
  }

  /**
   * Build URL with query parameters
   */
  buildUrl(endpoint, params = {}) {
    const url = new URL(endpoint, this.apiUrl);
    Object.entries(params).forEach(([key, value]) => {
      if (value !== undefined && value !== null && value !== "") {
        url.searchParams.append(key, value.toString());
      }
    });
    return url.toString();
  }
}

/**
 * Health and Status Endpoints
 */
class HealthEndpoints {
  constructor(client) {
    this.client = client;
  }

  async health() {
    return this.client.call("/health");
  }

  async liveness() {
    return this.client.call("/liveness");
  }

  async status() {
    return this.client.call("/status");
  }

  async version() {
    return this.client.call("/version");
  }

  async metrics() {
    return this.client.call("/metrics");
  }
}

/**
 * Scan Management Endpoints
 */
class ScanEndpoints {
  constructor(client) {
    this.client = client;
    this.activeMonitors = new Map();
  }

  /**
   * List scans with optional filtering and pagination
   */
  async list(filters = {}, pagination = {}) {
    const params = {
      page: pagination.page || 1,
      page_size: pagination.pageSize || 20,
      ...filters,
    };

    const url = this.client.buildUrl("/scans", params);
    return this.client.callWithRetry(url);
  }

  /**
   * Create a new scan
   */
  async create(scanData) {
    const requiredFields = ["name", "targets", "scan_type"];
    for (const field of requiredFields) {
      if (!scanData[field]) {
        throw new Error(`Missing required field: ${field}`);
      }
    }

    return this.client.callWithRetry("/scans", {
      method: "POST",
      body: JSON.stringify(scanData),
    });
  }

  /**
   * Get a specific scan
   */
  async get(scanId) {
    return this.client.callWithRetry(`/scans/${scanId}`);
  }

  /**
   * Update a scan
   */
  async update(scanId, updateData) {
    return this.client.callWithRetry(`/scans/${scanId}`, {
      method: "PUT",
      body: JSON.stringify(updateData),
    });
  }

  /**
   * Delete a scan
   */
  async delete(scanId) {
    return this.client.callWithRetry(`/scans/${scanId}`, {
      method: "DELETE",
    });
  }

  /**
   * Start a scan
   */
  async start(scanId) {
    return this.client.callWithRetry(`/scans/${scanId}/start`, {
      method: "POST",
    });
  }

  /**
   * Stop a scan
   */
  async stop(scanId) {
    return this.client.callWithRetry(`/scans/${scanId}/stop`, {
      method: "POST",
    });
  }

  /**
   * Get scan results with pagination
   */
  async getResults(scanId, pagination = {}) {
    const params = {
      page: pagination.page || 1,
      page_size: pagination.pageSize || 20,
    };

    const url = this.client.buildUrl(`/scans/${scanId}/results`, params);
    return this.client.callWithRetry(url);
  }

  /**
   * Monitor scan progress in real-time
   */
  async monitor(scanId, onUpdate, onComplete, onError) {
    // Stop any existing monitor for this scan
    this.stopMonitoring(scanId);

    const poll = async () => {
      try {
        const scan = await this.get(scanId);

        if (onUpdate) onUpdate(scan);

        if (scan.status === "running" || scan.status === "pending") {
          const timeoutId = setTimeout(poll, 2000);
          this.activeMonitors.set(scanId, timeoutId);
        } else {
          this.stopMonitoring(scanId);
          if (onComplete) onComplete(scan);
        }
      } catch (error) {
        this.stopMonitoring(scanId);
        if (onError) onError(error);
      }
    };

    poll();
  }

  /**
   * Stop monitoring a scan
   */
  stopMonitoring(scanId) {
    const timeoutId = this.activeMonitors.get(scanId);
    if (timeoutId) {
      clearTimeout(timeoutId);
      this.activeMonitors.delete(scanId);
    }
  }

  /**
   * Stop all scan monitoring
   */
  stopAllMonitoring() {
    for (const [scanId] of this.activeMonitors) {
      this.stopMonitoring(scanId);
    }
  }
}

/**
 * Host Management Endpoints
 */
class HostEndpoints {
  constructor(client) {
    this.client = client;
  }

  /**
   * List hosts with optional filtering and pagination
   */
  async list(filters = {}, pagination = {}) {
    const params = {
      page: pagination.page || 1,
      page_size: pagination.pageSize || 20,
      ...filters,
    };

    const url = this.client.buildUrl("/hosts", params);
    return this.client.callWithRetry(url);
  }

  /**
   * Create a new host
   */
  async create(hostData) {
    if (!hostData.ip) {
      throw new Error("IP address is required");
    }

    return this.client.callWithRetry("/hosts", {
      method: "POST",
      body: JSON.stringify(hostData),
    });
  }

  /**
   * Get a specific host
   */
  async get(hostId) {
    return this.client.callWithRetry(`/hosts/${hostId}`);
  }

  /**
   * Update a host
   */
  async update(hostId, updateData) {
    return this.client.callWithRetry(`/hosts/${hostId}`, {
      method: "PUT",
      body: JSON.stringify(updateData),
    });
  }

  /**
   * Delete a host
   */
  async delete(hostId) {
    return this.client.callWithRetry(`/hosts/${hostId}`, {
      method: "DELETE",
    });
  }

  /**
   * Get scans for a specific host
   */
  async getScans(hostId, pagination = {}) {
    const params = {
      page: pagination.page || 1,
      page_size: pagination.pageSize || 20,
    };

    const url = this.client.buildUrl(`/hosts/${hostId}/scans`, params);
    return this.client.callWithRetry(url);
  }

  /**
   * Get hosts by network
   */
  async getByNetwork(network, pagination = {}) {
    return this.list({ network }, pagination);
  }

  /**
   * Get hosts by OS
   */
  async getByOS(os, pagination = {}) {
    return this.list({ os }, pagination);
  }

  /**
   * Get active hosts only
   */
  async getActive(pagination = {}) {
    return this.list({ status: "up" }, pagination);
  }
}

/**
 * Profile Management Endpoints
 */
class ProfileEndpoints {
  constructor(client) {
    this.client = client;
  }

  /**
   * List profiles with optional filtering and pagination
   */
  async list(filters = {}, pagination = {}) {
    const params = {
      page: pagination.page || 1,
      page_size: pagination.pageSize || 20,
      ...filters,
    };

    const url = this.client.buildUrl("/profiles", params);
    return this.client.callWithRetry(url);
  }

  /**
   * Create a new profile
   */
  async create(profileData) {
    const requiredFields = ["name", "scan_type"];
    for (const field of requiredFields) {
      if (!profileData[field]) {
        throw new Error(`Missing required field: ${field}`);
      }
    }

    return this.client.callWithRetry("/profiles", {
      method: "POST",
      body: JSON.stringify(profileData),
    });
  }

  /**
   * Get a specific profile
   */
  async get(profileId) {
    return this.client.callWithRetry(`/profiles/${profileId}`);
  }

  /**
   * Update a profile
   */
  async update(profileId, updateData) {
    return this.client.callWithRetry(`/profiles/${profileId}`, {
      method: "PUT",
      body: JSON.stringify(updateData),
    });
  }

  /**
   * Delete a profile
   */
  async delete(profileId) {
    return this.client.callWithRetry(`/profiles/${profileId}`, {
      method: "DELETE",
    });
  }

  /**
   * Get profiles by scan type
   */
  async getByScanType(scanType, pagination = {}) {
    return this.list({ scan_type: scanType }, pagination);
  }

  /**
   * Get default profiles
   */
  async getDefaults(pagination = {}) {
    return this.list({ default: true }, pagination);
  }
}

/**
 * Discovery Management Endpoints
 */
class DiscoveryEndpoints {
  constructor(client) {
    this.client = client;
  }

  /**
   * List discovery jobs
   */
  async list(filters = {}, pagination = {}) {
    const params = {
      page: pagination.page || 1,
      page_size: pagination.pageSize || 20,
      ...filters,
    };

    const url = this.client.buildUrl("/discovery", params);
    return this.client.callWithRetry(url);
  }

  /**
   * Create a discovery job
   */
  async create(discoveryData) {
    return this.client.callWithRetry("/discovery", {
      method: "POST",
      body: JSON.stringify(discoveryData),
    });
  }

  /**
   * Get a specific discovery job
   */
  async get(jobId) {
    return this.client.callWithRetry(`/discovery/${jobId}`);
  }

  /**
   * Start a discovery job
   */
  async start(jobId) {
    return this.client.callWithRetry(`/discovery/${jobId}/start`, {
      method: "POST",
    });
  }

  /**
   * Stop a discovery job
   */
  async stop(jobId) {
    return this.client.callWithRetry(`/discovery/${jobId}/stop`, {
      method: "POST",
    });
  }
}

/**
 * Schedule Management Endpoints
 */
class ScheduleEndpoints {
  constructor(client) {
    this.client = client;
  }

  /**
   * List schedules
   */
  async list(filters = {}, pagination = {}) {
    const params = {
      page: pagination.page || 1,
      page_size: pagination.pageSize || 20,
      ...filters,
    };

    const url = this.client.buildUrl("/schedules", params);
    return this.client.callWithRetry(url);
  }

  /**
   * Create a schedule
   */
  async create(scheduleData) {
    return this.client.callWithRetry("/schedules", {
      method: "POST",
      body: JSON.stringify(scheduleData),
    });
  }

  /**
   * Get a specific schedule
   */
  async get(scheduleId) {
    return this.client.callWithRetry(`/schedules/${scheduleId}`);
  }

  /**
   * Update a schedule
   */
  async update(scheduleId, updateData) {
    return this.client.callWithRetry(`/schedules/${scheduleId}`, {
      method: "PUT",
      body: JSON.stringify(updateData),
    });
  }

  /**
   * Delete a schedule
   */
  async delete(scheduleId) {
    return this.client.callWithRetry(`/schedules/${scheduleId}`, {
      method: "DELETE",
    });
  }

  /**
   * Enable a schedule
   */
  async enable(scheduleId) {
    return this.client.callWithRetry(`/schedules/${scheduleId}/enable`, {
      method: "POST",
    });
  }

  /**
   * Disable a schedule
   */
  async disable(scheduleId) {
    return this.client.callWithRetry(`/schedules/${scheduleId}/disable`, {
      method: "POST",
    });
  }
}

/**
 * Utility class for paginated data handling
 */
class PaginatedData {
  constructor(data, pagination) {
    this.data = data;
    this.pagination = pagination;
  }

  get items() {
    return this.data;
  }

  get totalItems() {
    return this.pagination.total_items;
  }

  get totalPages() {
    return this.pagination.total_pages;
  }

  get currentPage() {
    return this.pagination.page;
  }

  get pageSize() {
    return this.pagination.page_size;
  }

  get hasNextPage() {
    return this.currentPage < this.totalPages;
  }

  get hasPreviousPage() {
    return this.currentPage > 1;
  }
}

/**
 * Enhanced endpoints with pagination helpers
 */
class BaseEndpoints {
  constructor(client, basePath) {
    this.client = client;
    this.basePath = basePath;
  }

  /**
   * List with automatic pagination wrapper
   */
  async listPaginated(filters = {}, pagination = {}) {
    const response = await this.list(filters, pagination);
    return new PaginatedData(response.data, response.pagination);
  }

  /**
   * Get all items across all pages
   */
  async listAll(filters = {}, maxPages = 100) {
    const allItems = [];
    let page = 1;

    while (page <= maxPages) {
      const response = await this.list(filters, { page, page_size: 100 });
      allItems.push(...response.data);

      if (page >= response.pagination.total_pages) {
        break;
      }
      page++;
    }

    return allItems;
  }
}

// Extend scan endpoints with pagination helpers
Object.setPrototypeOf(ScanEndpoints.prototype, BaseEndpoints.prototype);
Object.setPrototypeOf(HostEndpoints.prototype, BaseEndpoints.prototype);
Object.setPrototypeOf(ProfileEndpoints.prototype, BaseEndpoints.prototype);

/**
 * Real-time monitoring utilities
 */
class ScanMonitor {
  constructor(client) {
    this.client = client;
    this.activeMonitors = new Map();
  }

  /**
   * Monitor multiple scans simultaneously
   */
  monitorMultiple(scanIds, callbacks = {}) {
    const results = new Map();

    scanIds.forEach((scanId) => {
      this.client.scans.monitor(
        scanId,
        (scan) => {
          results.set(scanId, scan);
          if (callbacks.onUpdate) {
            callbacks.onUpdate(scanId, scan, results);
          }
        },
        (scan) => {
          results.set(scanId, scan);
          if (callbacks.onComplete) {
            callbacks.onComplete(scanId, scan, results);
          }
        },
        (error) => {
          if (callbacks.onError) {
            callbacks.onError(scanId, error);
          }
        },
      );
    });

    return results;
  }

  /**
   * Get progress summary for multiple scans
   */
  getProgressSummary(scanResults) {
    const summary = {
      total: scanResults.size,
      pending: 0,
      running: 0,
      completed: 0,
      failed: 0,
      averageProgress: 0,
    };

    let totalProgress = 0;
    for (const scan of scanResults.values()) {
      summary[scan.status]++;
      totalProgress += scan.progress || 0;
    }

    summary.averageProgress =
      summary.total > 0 ? totalProgress / summary.total : 0;
    return summary;
  }
}

/**
 * Error handling utilities
 */
class ApiError extends Error {
  constructor(message, status, response, data) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.response = response;
    this.data = data;
  }

  get isClientError() {
    return this.status >= 400 && this.status < 500;
  }

  get isServerError() {
    return this.status >= 500;
  }

  get isNetworkError() {
    return !this.status;
  }
}

/**
 * Data validation utilities
 */
class Validators {
  static isValidIP(ip) {
    const ipv4Regex =
      /^(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)$/;
    const ipv6Regex = /^(?:[0-9a-fA-F]{1,4}:){7}[0-9a-fA-F]{1,4}$/;
    return ipv4Regex.test(ip) || ipv6Regex.test(ip);
  }

  static isValidCIDR(cidr) {
    const parts = cidr.split("/");
    if (parts.length !== 2) return false;

    const [ip, mask] = parts;
    if (!this.isValidIP(ip)) return false;

    const maskNum = parseInt(mask, 10);
    return maskNum >= 0 && maskNum <= 32; // Simplified for IPv4
  }

  static isValidPortRange(ports) {
    const portRegex = /^(\d{1,5}(-\d{1,5})?,?)+$/;
    if (!portRegex.test(ports.replace(/\s/g, ""))) return false;

    const ranges = ports.split(",");
    return ranges.every((range) => {
      const [start, end] = range.split("-").map((p) => parseInt(p.trim(), 10));
      return (
        start >= 1 && start <= 65535 && (!end || (end >= start && end <= 65535))
      );
    });
  }

  static validateScanData(scanData) {
    const errors = [];

    if (!scanData.name) errors.push("Name is required");
    if (!scanData.targets || scanData.targets.length === 0)
      errors.push("At least one target is required");
    if (!scanData.scan_type) errors.push("Scan type is required");

    if (scanData.targets) {
      scanData.targets.forEach((target, index) => {
        if (!this.isValidIP(target) && !this.isValidCIDR(target)) {
          errors.push(`Target ${index + 1} is not a valid IP or CIDR`);
        }
      });
    }

    if (scanData.ports && !this.isValidPortRange(scanData.ports)) {
      errors.push("Invalid port range format");
    }

    return errors;
  }

  static validateHostData(hostData) {
    const errors = [];

    if (!hostData.ip) {
      errors.push("IP address is required");
    } else if (!this.isValidIP(hostData.ip)) {
      errors.push("Invalid IP address format");
    }

    if (hostData.hostname && hostData.hostname.length > 255) {
      errors.push("Hostname too long (max 255 characters)");
    }

    if (hostData.description && hostData.description.length > 1000) {
      errors.push("Description too long (max 1000 characters)");
    }

    return errors;
  }
}

/**
 * React hooks for easy integration
 */
const ScanoramaHooks = {
  /**
   * Hook for managing scans
   */
  useScans: (client, initialFilters = {}) => {
    if (typeof React === "undefined") {
      throw new Error("React hooks require React to be available");
    }

    const [scans, setScans] = React.useState([]);
    const [loading, setLoading] = React.useState(true);
    const [error, setError] = React.useState(null);
    const [pagination, setPagination] = React.useState({});
    const [filters, setFilters] = React.useState(initialFilters);

    const loadScans = React.useCallback(
      async (page = 1) => {
        try {
          setLoading(true);
          setError(null);
          const response = await client.scans.list(filters, { page });
          setScans(response.data);
          setPagination(response.pagination);
        } catch (err) {
          setError(err.message);
        } finally {
          setLoading(false);
        }
      },
      [client, filters],
    );

    React.useEffect(() => {
      loadScans();
    }, [loadScans]);

    const createScan = React.useCallback(
      async (scanData) => {
        const validation = Validators.validateScanData(scanData);
        if (validation.length > 0) {
          throw new Error(validation.join(", "));
        }

        const scan = await client.scans.create(scanData);
        await loadScans(); // Refresh list
        return scan;
      },
      [client, loadScans],
    );

    const startScan = React.useCallback(
      async (scanId) => {
        await client.scans.start(scanId);
        await loadScans(); // Refresh list
      },
      [client, loadScans],
    );

    const stopScan = React.useCallback(
      async (scanId) => {
        await client.scans.stop(scanId);
        await loadScans(); // Refresh list
      },
      [client, loadScans],
    );

    return {
      scans,
      loading,
      error,
      pagination,
      filters,
      setFilters,
      loadScans,
      createScan,
      startScan,
      stopScan,
      refetch: () => loadScans(pagination.page),
    };
  },

  /**
   * Hook for managing hosts
   */
  useHosts: (client, initialFilters = {}) => {
    if (typeof React === "undefined") {
      throw new Error("React hooks require React to be available");
    }

    const [hosts, setHosts] = React.useState([]);
    const [loading, setLoading] = React.useState(true);
    const [error, setError] = React.useState(null);
    const [pagination, setPagination] = React.useState({});
    const [filters, setFilters] = React.useState(initialFilters);

    const loadHosts = React.useCallback(
      async (page = 1) => {
        try {
          setLoading(true);
          setError(null);
          const response = await client.hosts.list(filters, { page });
          setHosts(response.data);
          setPagination(response.pagination);
        } catch (err) {
          setError(err.message);
        } finally {
          setLoading(false);
        }
      },
      [client, filters],
    );

    React.useEffect(() => {
      loadHosts();
    }, [loadHosts]);

    return {
      hosts,
      loading,
      error,
      pagination,
      filters,
      setFilters,
      loadHosts,
      refetch: () => loadHosts(pagination.page),
    };
  },
};

/**
 * Vue.js composables
 */
const ScanoramaComposables = {
  /**
   * Composable for scan management
   */
  useScans: (client, initialFilters = {}) => {
    if (typeof Vue === "undefined") {
      throw new Error("Vue composables require Vue to be available");
    }

    const scans = Vue.ref([]);
    const loading = Vue.ref(true);
    const error = Vue.ref(null);
    const pagination = Vue.ref({});
    const filters = Vue.ref(initialFilters);

    const loadScans = async (page = 1) => {
      try {
        loading.value = true;
        error.value = null;
        const response = await client.scans.list(filters.value, { page });
        scans.value = response.data;
        pagination.value = response.pagination;
      } catch (err) {
        error.value = err.message;
      } finally {
        loading.value = false;
      }
    };

    Vue.watch(filters, () => loadScans(), { deep: true });
    Vue.onMounted(() => loadScans());

    return {
      scans,
      loading,
      error,
      pagination,
      filters,
      loadScans,
      refetch: () => loadScans(pagination.value.page),
    };
  },
};

/**
 * Example usage and testing utilities
 */
const ExampleUsage = {
  /**
   * Demo all main functionality
   */
  async demo() {
    const client = new ScanoramaClient();

    try {
      // Check API health
      console.log("🔍 Checking API health...");
      const health = await client.health.health();
      console.log("✅ API is healthy:", health);

      // List existing scans
      console.log("📋 Fetching scans...");
      const scansResponse = await client.scans.list();
      console.log(`✅ Found ${scansResponse.data.length} scans`);

      // List hosts
      console.log("🖥️  Fetching hosts...");
      const hostsResponse = await client.hosts.list();
      console.log(`✅ Found ${hostsResponse.data.length} hosts`);

      // List profiles
      console.log("⚙️  Fetching profiles...");
      const profilesResponse = await client.profiles.list();
      console.log(`✅ Found ${profilesResponse.data.length} profiles`);

      // Create a test scan
      console.log("🚀 Creating test scan...");
      const newScan = await client.scans.create({
        name: "API Test Scan",
        description: "Test scan created via API client",
        targets: ["127.0.0.1"],
        scan_type: "connect",
        ports: "22,80,443",
        options: { timing: "normal" },
        tags: ["test", "api-client"],
      });
      console.log("✅ Test scan created:", newScan.id);

      return {
        health,
        scansCount: scansResponse.data.length,
        hostsCount: hostsResponse.data.length,
        profilesCount: profilesResponse.data.length,
        testScanId: newScan.id,
      };
    } catch (error) {
      console.error("❌ Demo failed:", error);
      throw error;
    }
  },

  /**
   * Test scan monitoring
   */
  async testScanMonitoring(client, scanId) {
    console.log(`🔄 Starting monitoring for scan ${scanId}...`);

    return new Promise((resolve, reject) => {
      const startTime = Date.now();

      client.scans.monitor(
        scanId,
        (scan) => {
          const elapsed = ((Date.now() - startTime) / 1000).toFixed(1);
          console.log(
            `📊 [${elapsed}s] ${scan.name}: ${scan.status} (${scan.progress || 0}%)`,
          );
        },
        (scan) => {
          const elapsed = ((Date.now() - startTime) / 1000).toFixed(1);
          console.log(`✅ [${elapsed}s] Scan completed: ${scan.status}`);
          resolve(scan);
        },
        (error) => {
          console.error("❌ Monitoring failed:", error);
          reject(error);
        },
      );
    });
  },

  /**
   * Test error handling
   */
  async testErrorHandling(client) {
    console.log("🧪 Testing error handling...");

    try {
      // Test 404 error
      await client.scans.get("non-existent-id");
    } catch (error) {
      console.log("✅ 404 handling works:", error.message);
    }

    try {
      // Test validation error
      await client.scans.create({});
    } catch (error) {
      console.log("✅ Validation error handling works:", error.message);
    }

    console.log("✅ Error handling tests completed");
  },
};

/**
 * Export for different module systems
 */
if (typeof module !== "undefined" && module.exports) {
  // Node.js
  module.exports = {
    ScanoramaClient,
    ScanMonitor,
    ApiError,
    Validators,
    PaginatedData,
    ExampleUsage,
  };
} else if (typeof window !== "undefined") {
  // Browser
  window.ScanoramaClient = ScanoramaClient;
  window.ScanMonitor = ScanMonitor;
  window.ApiError = ApiError;
  window.Validators = Validators;
  window.PaginatedData = PaginatedData;
  window.ExampleUsage = ExampleUsage;
  window.ScanoramaHooks = ScanoramaHooks;
  window.ScanoramaComposables = ScanoramaComposables;
}

// ES6 exports (for modern bundlers)
export {
  ScanoramaClient,
  ScanMonitor,
  ApiError,
  Validators,
  PaginatedData,
  ExampleUsage,
  ScanoramaHooks,
  ScanoramaComposables,
};

export default ScanoramaClient;
