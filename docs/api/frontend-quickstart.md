# Frontend Developer Quick Start Guide

## Getting Started with Scanorama API

This guide helps frontend developers quickly integrate with the Scanorama API. All examples use the confirmed working endpoints from our integration testing.

## Setup

### 1. Start the Development Environment

```bash
# Start the database
docker-compose up -d postgres

# Build and run the API server
go build -o scanorama ./cmd/scanorama
./scanorama api --config config.local.yaml
```

The API will be available at: `http://localhost:8080`

### 2. Verify API is Running

```bash
curl http://localhost:8080/api/v1/health
```

Expected response:
```json
{
  "status": "healthy",
  "database": "connected",
  "timestamp": "2025-01-14T00:20:30Z"
}
```

## Core API Patterns

### Base Configuration

```javascript
const API_BASE_URL = 'http://localhost:8080/api/v1';

const apiCall = async (endpoint, options = {}) => {
  const response = await fetch(`${API_BASE_URL}${endpoint}`, {
    headers: {
      'Content-Type': 'application/json',
      ...options.headers
    },
    ...options
  });

  if (!response.ok) {
    const error = await response.json();
    throw new Error(error.error || `HTTP ${response.status}`);
  }

  return response.json();
};
```

## Working Examples

### 1. List Scans (✅ Confirmed Working)

```javascript
// Fetch all scans
const fetchScans = async () => {
  try {
    const data = await apiCall('/scans');
    return data.data; // Array of scan objects
  } catch (error) {
    console.error('Failed to fetch scans:', error);
    throw error;
  }
};

// Fetch scans with filtering
const fetchFilteredScans = async (filters = {}) => {
  const params = new URLSearchParams(filters);
  const data = await apiCall(`/scans?${params}`);
  return data.data;
};

// Usage examples
const allScans = await fetchScans();
const runningScans = await fetchFilteredScans({ status: 'running' });
const connectScans = await fetchFilteredScans({ scan_type: 'connect' });
```

### 2. Create Scan (✅ Confirmed Working)

```javascript
const createScan = async (scanData) => {
  const scan = await apiCall('/scans', {
    method: 'POST',
    body: JSON.stringify(scanData)
  });
  return scan;
};

// Example usage
const newScan = await createScan({
  name: 'Production Network Scan',
  description: 'Weekly security scan of production network',
  targets: ['192.168.1.0/24'],
  scan_type: 'connect',
  ports: '22,80,443,8080,8443',
  options: {
    timing: 'normal'
  },
  tags: ['production', 'weekly']
});
```

### 3. Monitor Scan Progress

```javascript
const monitorScan = async (scanId) => {
  const scan = await apiCall(`/scans/${scanId}`);
  return {
    id: scan.id,
    name: scan.name,
    status: scan.status,
    progress: scan.progress || 0,
    duration: scan.duration
  };
};

// Poll for updates
const pollScanProgress = (scanId, onUpdate, interval = 2000) => {
  const poll = async () => {
    try {
      const scan = await monitorScan(scanId);
      onUpdate(scan);
      
      if (scan.status === 'running') {
        setTimeout(poll, interval);
      }
    } catch (error) {
      console.error('Failed to poll scan:', error);
    }
  };
  
  poll();
};
```

### 4. Get Scan Results

```javascript
const getScanResults = async (scanId, page = 1, pageSize = 20) => {
  const data = await apiCall(`/scans/${scanId}/results?page=${page}&page_size=${pageSize}`);
  return {
    results: data.results || [],
    totalHosts: data.total_hosts || 0,
    totalPorts: data.total_ports || 0,
    openPorts: data.open_ports || 0,
    summary: data.summary || {}
  };
};
```

### 5. List Hosts (✅ Confirmed Working)

```javascript
const fetchHosts = async (filters = {}) => {
  const params = new URLSearchParams(filters);
  const data = await apiCall(`/hosts?${params}`);
  return data.data;
};

// Filter examples
const linuxHosts = await fetchHosts({ os: 'linux' });
const activeHosts = await fetchHosts({ status: 'up' });
const networkHosts = await fetchHosts({ network: '192.168.1.0/24' });
```

### 6. List Profiles (✅ Confirmed Working)

```javascript
const fetchProfiles = async () => {
  const data = await apiCall('/profiles');
  return data.data;
};

const getProfilesForScanType = async (scanType) => {
  const data = await apiCall(`/profiles?scan_type=${scanType}`);
  return data.data;
};
```

## React Components Examples

### Scan List Component

```jsx
import React, { useState, useEffect } from 'react';

const ScanList = () => {
  const [scans, setScans] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  useEffect(() => {
    loadScans();
  }, []);

  const loadScans = async () => {
    try {
      setLoading(true);
      const data = await apiCall('/scans');
      setScans(data.data);
    } catch (err) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  const handleStartScan = async (scanId) => {
    try {
      await apiCall(`/scans/${scanId}/start`, { method: 'POST' });
      await loadScans(); // Refresh
    } catch (err) {
      alert(`Failed to start scan: ${err.message}`);
    }
  };

  if (loading) return <div>Loading scans...</div>;
  if (error) return <div>Error: {error}</div>;

  return (
    <div>
      <h2>Network Scans</h2>
      <table>
        <thead>
          <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Status</th>
            <th>Progress</th>
            <th>Actions</th>
          </tr>
        </thead>
        <tbody>
          {scans.map(scan => (
            <tr key={scan.id}>
              <td>{scan.name}</td>
              <td>{scan.scan_type}</td>
              <td>
                <span className={`status-${scan.status}`}>
                  {scan.status}
                </span>
              </td>
              <td>
                {scan.status === 'running' ? (
                  <div className="progress-bar">
                    <div 
                      className="progress-fill" 
                      style={{ width: `${scan.progress || 0}%` }}
                    />
                  </div>
                ) : (
                  scan.status === 'completed' ? '100%' : '0%'
                )}
              </td>
              <td>
                {scan.status === 'pending' && (
                  <button onClick={() => handleStartScan(scan.id)}>
                    Start
                  </button>
                )}
                <button onClick={() => window.open(`/scans/${scan.id}/results`)}>
                  Results
                </button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
};
```

### Host Dashboard Component

```jsx
import React, { useState, useEffect } from 'react';

const HostDashboard = () => {
  const [hosts, setHosts] = useState([]);
  const [filters, setFilters] = useState({});
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    loadHosts();
  }, [filters]);

  const loadHosts = async () => {
    try {
      setLoading(true);
      const params = new URLSearchParams(filters);
      const data = await apiCall(`/hosts?${params}`);
      setHosts(data.data);
    } catch (err) {
      console.error('Failed to load hosts:', err);
    } finally {
      setLoading(false);
    }
  };

  const updateFilter = (key, value) => {
    setFilters(prev => ({
      ...prev,
      [key]: value || undefined
    }));
  };

  return (
    <div>
      <h2>Network Hosts</h2>
      
      {/* Filters */}
      <div className="filters">
        <select 
          value={filters.os || ''} 
          onChange={(e) => updateFilter('os', e.target.value)}
        >
          <option value="">All OS</option>
          <option value="linux">Linux</option>
          <option value="windows">Windows</option>
          <option value="macos">macOS</option>
        </select>
        
        <select 
          value={filters.status || ''} 
          onChange={(e) => updateFilter('status', e.target.value)}
        >
          <option value="">All Status</option>
          <option value="up">Up</option>
          <option value="down">Down</option>
        </select>
      </div>

      {/* Host Grid */}
      {loading ? (
        <div>Loading hosts...</div>
      ) : (
        <div className="host-grid">
          {hosts.map(host => (
            <div key={host.id} className="host-card">
              <h3>{host.hostname || host.ip}</h3>
              <p>IP: {host.ip}</p>
              {host.os && <p>OS: {host.os} {host.os_version}</p>}
              <p>
                Status: 
                <span className={`status-${host.active ? 'up' : 'down'}`}>
                  {host.active ? 'Up' : 'Down'}
                </span>
              </p>
              <p>Open Ports: {host.open_ports}/{host.total_ports}</p>
              <p>Scans: {host.scan_count}</p>
              {host.last_seen && (
                <p>Last Seen: {new Date(host.last_seen).toLocaleString()}</p>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  );
};
```

## Vue.js Examples

### Scan Creation Form

```vue
<template>
  <div class="scan-form">
    <h2>Create New Scan</h2>
    <form @submit.prevent="createScan">
      <div class="form-group">
        <label for="name">Scan Name</label>
        <input 
          id="name"
          v-model="scanData.name" 
          type="text" 
          required 
          maxlength="255"
        />
      </div>

      <div class="form-group">
        <label for="description">Description</label>
        <textarea 
          id="description"
          v-model="scanData.description"
          maxlength="1000"
        ></textarea>
      </div>

      <div class="form-group">
        <label for="targets">Targets (one per line)</label>
        <textarea 
          id="targets"
          v-model="targetsText"
          placeholder="192.168.1.0/24&#10;10.0.0.1&#10;example.com"
          required
        ></textarea>
      </div>

      <div class="form-group">
        <label for="scanType">Scan Type</label>
        <select id="scanType" v-model="scanData.scan_type" required>
          <option value="connect">TCP Connect</option>
          <option value="syn">SYN Stealth</option>
          <option value="version">Version Detection</option>
          <option value="aggressive">Aggressive</option>
          <option value="comprehensive">Comprehensive</option>
        </select>
      </div>

      <div class="form-group">
        <label for="ports">Ports</label>
        <input 
          id="ports"
          v-model="scanData.ports" 
          type="text" 
          placeholder="22,80,443,8000-9000"
        />
      </div>

      <div class="form-group">
        <label for="profile">Profile</label>
        <select id="profile" v-model="scanData.profile_id">
          <option value="">Default</option>
          <option 
            v-for="profile in profiles" 
            :key="profile.id" 
            :value="profile.id"
          >
            {{ profile.name }}
          </option>
        </select>
      </div>

      <button type="submit" :disabled="submitting">
        {{ submitting ? 'Creating...' : 'Create Scan' }}
      </button>
    </form>
  </div>
</template>

<script>
import { ref, computed, onMounted } from 'vue';

export default {
  name: 'ScanCreationForm',
  setup() {
    const scanData = ref({
      name: '',
      description: '',
      scan_type: 'connect',
      ports: '22,80,443',
      profile_id: '',
      options: {
        timing: 'normal'
      },
      tags: []
    });
    
    const targetsText = ref('');
    const profiles = ref([]);
    const submitting = ref(false);

    const targets = computed(() => 
      targetsText.value
        .split('\n')
        .map(t => t.trim())
        .filter(t => t.length > 0)
    );

    const loadProfiles = async () => {
      try {
        const response = await fetch('/api/v1/profiles');
        const data = await response.json();
        profiles.value = data.data;
      } catch (error) {
        console.error('Failed to load profiles:', error);
      }
    };

    const createScan = async () => {
      if (targets.value.length === 0) {
        alert('Please enter at least one target');
        return;
      }

      try {
        submitting.value = true;
        
        const scanPayload = {
          ...scanData.value,
          targets: targets.value
        };

        const response = await fetch('/api/v1/scans', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(scanPayload)
        });

        if (!response.ok) {
          throw new Error('Failed to create scan');
        }

        const result = await response.json();
        alert(`Scan created successfully: ${result.name}`);
        
        // Reset form
        Object.assign(scanData.value, {
          name: '',
          description: '',
          scan_type: 'connect',
          ports: '22,80,443',
          profile_id: ''
        });
        targetsText.value = '';
        
      } catch (error) {
        alert(`Error creating scan: ${error.message}`);
      } finally {
        submitting.value = false;
      }
    };

    onMounted(() => {
      loadProfiles();
    });

    return {
      scanData,
      targetsText,
      profiles,
      submitting,
      createScan
    };
  }
};
</script>
```

### Real-time Scan Monitoring

```javascript
class ScanMonitor {
  constructor() {
    this.activePolls = new Map();
  }

  // Start monitoring a scan
  startMonitoring(scanId, onUpdate, onComplete) {
    if (this.activePolls.has(scanId)) {
      this.stopMonitoring(scanId);
    }

    const poll = async () => {
      try {
        const scan = await apiCall(`/scans/${scanId}`);
        
        onUpdate(scan);

        if (scan.status === 'running') {
          const timeoutId = setTimeout(poll, 2000);
          this.activePolls.set(scanId, timeoutId);
        } else {
          this.stopMonitoring(scanId);
          if (onComplete) onComplete(scan);
        }
      } catch (error) {
        console.error(`Failed to monitor scan ${scanId}:`, error);
        this.stopMonitoring(scanId);
      }
    };

    poll();
  }

  // Stop monitoring a scan
  stopMonitoring(scanId) {
    const timeoutId = this.activePolls.get(scanId);
    if (timeoutId) {
      clearTimeout(timeoutId);
      this.activePolls.delete(scanId);
    }
  }

  // Stop all monitoring
  stopAll() {
    for (const [scanId] of this.activePolls) {
      this.stopMonitoring(scanId);
    }
  }
}

// Usage
const monitor = new ScanMonitor();

monitor.startMonitoring(
  'scan-uuid',
  (scan) => {
    console.log(`Scan ${scan.name}: ${scan.status} (${scan.progress}%)`);
    updateProgressBar(scan.progress);
  },
  (scan) => {
    console.log(`Scan ${scan.name} completed`);
    loadScanResults(scan.id);
  }
);
```

## Data Tables with Pagination

### Host Table with Filtering

```javascript
// React component for host management
const HostTable = () => {
  const [hosts, setHosts] = useState([]);
  const [pagination, setPagination] = useState({});
  const [filters, setFilters] = useState({});
  const [currentPage, setCurrentPage] = useState(1);
  
  const loadHosts = async (page = 1) => {
    try {
      const params = new URLSearchParams({
        page: page.toString(),
        page_size: '20',
        ...filters
      });
      
      const response = await apiCall(`/hosts?${params}`);
      setHosts(response.data);
      setPagination(response.pagination);
      setCurrentPage(page);
    } catch (error) {
      console.error('Failed to load hosts:', error);
    }
  };

  const handleFilterChange = (newFilters) => {
    setFilters(newFilters);
    setCurrentPage(1);
  };

  useEffect(() => {
    loadHosts(currentPage);
  }, [filters, currentPage]);

  return (
    <div>
      {/* Filter Controls */}
      <div className="filters">
        <select 
          value={filters.os || ''} 
          onChange={(e) => handleFilterChange({...filters, os: e.target.value})}
        >
          <option value="">All Operating Systems</option>
          <option value="linux">Linux</option>
          <option value="windows">Windows</option>
          <option value="macos">macOS</option>
        </select>
        
        <select 
          value={filters.status || ''} 
          onChange={(e) => handleFilterChange({...filters, status: e.target.value})}
        >
          <option value="">All Status</option>
          <option value="up">Up</option>
          <option value="down">Down</option>
        </select>
      </div>

      {/* Host Table */}
      <table>
        <thead>
          <tr>
            <th>IP Address</th>
            <th>Hostname</th>
            <th>OS</th>
            <th>Status</th>
            <th>Open Ports</th>
            <th>Last Seen</th>
            <th>Actions</th>
          </tr>
        </thead>
        <tbody>
          {hosts.map(host => (
            <tr key={host.id}>
              <td>{host.ip}</td>
              <td>{host.hostname || '-'}</td>
              <td>{host.os || 'Unknown'}</td>
              <td>
                <span className={`status-${host.active ? 'up' : 'down'}`}>
                  {host.active ? 'Up' : 'Down'}
                </span>
              </td>
              <td>{host.open_ports}</td>
              <td>
                {host.last_seen 
                  ? new Date(host.last_seen).toLocaleString() 
                  : 'Never'
                }
              </td>
              <td>
                <button onClick={() => scanHost(host.id)}>Scan</button>
                <button onClick={() => viewDetails(host.id)}>Details</button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>

      {/* Pagination */}
      <div className="pagination">
        <button 
          disabled={currentPage <= 1}
          onClick={() => setCurrentPage(p => p - 1)}
        >
          Previous
        </button>
        
        <span>
          Page {currentPage} of {pagination.total_pages || 1}
        </span>
        
        <button 
          disabled={currentPage >= (pagination.total_pages || 1)}
          onClick={() => setCurrentPage(p => p + 1)}
        >
          Next
        </button>
      </div>
    </div>
  );
};
```

## Error Handling Best Practices

```javascript
// Centralized error handling
const handleApiError = (error, context = '') => {
  console.error(`API Error ${context}:`, error);
  
  // User-friendly error messages
  const userMessage = {
    400: 'Invalid request. Please check your input.',
    401: 'Authentication required.',
    403: 'Access denied.',
    404: 'Resource not found.',
    409: 'Resource already exists.',
    429: 'Rate limit exceeded. Please try again later.',
    500: 'Server error. Please try again later.'
  };

  const status = error.status || 500;
  return userMessage[status] || error.message || 'An unexpected error occurred';
};

// Enhanced API call with retry logic
const apiCallWithRetry = async (endpoint, options = {}, maxRetries = 3) => {
  let lastError;
  
  for (let attempt = 1; attempt <= maxRetries; attempt++) {
    try {
      return await apiCall(endpoint, options);
    } catch (error) {
      lastError = error;
      
      // Don't retry on client errors (4xx)
      if (error.status >= 400 && error.status < 500) {
        throw error;
      }
      
      // Wait before retry (exponential backoff)
      if (attempt < maxRetries) {
        await new Promise(resolve => 
          setTimeout(resolve, Math.pow(2, attempt) * 1000)
        );
      }
    }
  }
  
  throw lastError;
};
```

## WebSocket Integration (Future)

```javascript
// Placeholder for real-time updates
class ScanoramaWebSocket {
  constructor(url = 'ws://localhost:8080/api/v1/ws') {
    this.url = url;
    this.ws = null;
    this.listeners = new Map();
  }

  connect() {
    this.ws = new WebSocket(this.url);
    
    this.ws.onmessage = (event) => {
      const data = JSON.parse(event.data);
      this.handleMessage(data);
    };

    this.ws.onclose = () => {
      // Reconnect after delay
      setTimeout(() => this.connect(), 5000);
    };
  }

  subscribe(eventType, callback) {
    if (!this.listeners.has(eventType)) {
      this.listeners.set(eventType, []);
    }
    this.listeners.get(eventType).push(callback);
  }

  handleMessage(data) {
    const callbacks = this.listeners.get(data.type) || [];
    callbacks.forEach(callback => callback(data));
  }
}
```

## CSS Styling Examples

```css
/* Status indicators */
.status-pending { color: #ffa500; }
.status-running { color: #007bff; }
.status-completed { color: #28a745; }
.status-failed { color: #dc3545; }
.status-up { color: #28a745; }
.status-down { color: #dc3545; }

/* Progress bar */
.progress-bar {
  width: 100px;
  height: 10px;
  background-color: #e9ecef;
  border-radius: 5px;
  overflow: hidden;
}

.progress-fill {
  height: 100%;
  background-color: #007bff;
  transition: width 0.3s ease;
}

/* Host grid */
.host-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: 1rem;
  margin-top: 1rem;
}

.host-card {
  border: 1px solid #dee2e6;
  border-radius: 8px;
  padding: 1rem;
  background: white;
  box-shadow: 0 2px 4px rgba(0,0,0,0.1);
}
```

## Quick Testing

Use these curl commands to test the API:

```bash
# List all scans
curl -s http://localhost:8080/api/v1/scans | jq

# List all hosts
curl -s http://localhost:8080/api/v1/hosts | jq

# List profiles
curl -s http://localhost:8080/api/v1/profiles | jq

# Create a simple scan
curl -s -X POST http://localhost:8080/api/v1/scans \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Test Scan",
    "targets": ["127.0.0.1"],
    "scan_type": "connect",
    "ports": "22,80,443"
  }' | jq

# Check scan status
curl -s http://localhost:8080/api/v1/scans/{scan-id} | jq
```

## Troubleshooting

### Common Issues

1. **404 Errors**: Check that the API server is running and endpoints are correctly typed
2. **400 Validation Errors**: Verify required fields and data formats
3. **Database Connection**: Ensure PostgreSQL is running via docker-compose
4. **CORS Issues**: Add your frontend URL to the CORS origins in config

### Debug Mode

Run the API with debug logging:
```bash
./scanorama api --config config.local.yaml --verbose
```

### Database Issues

```bash
# Check database status
docker-compose exec postgres pg_isready -U scanorama -d scanorama

# View database logs
docker-compose logs postgres

# Connect to database directly
docker-compose exec postgres psql -U scanorama -d scanorama
```

## Next Steps

1. **Start with list endpoints** - They're fully working and provide good foundation
2. **Implement scan creation** - Works well, focus on validation handling
3. **Add progress monitoring** - Use the polling patterns shown above
4. **Build host management** - List functionality is solid
5. **Add dashboard views** - Combine multiple endpoints for overview

The API is ready for frontend development with the core CRUD operations working correctly!