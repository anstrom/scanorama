import axios, { AxiosInstance } from 'axios';

// API Response Types
export interface HealthResponse {
  status: string;
  checks: {
    database: string;
    [key: string]: string;
  };
  timestamp: string;
}

export interface StatusResponse {
  service: string;
  version: string;
  uptime: string;
  timestamp: string;
}

export interface PaginationInfo {
  page: number;
  page_size: number;
  total_items: number;
  total_pages: number;
}

export interface PaginatedResponse<T> {
  data: T[];
  pagination: PaginationInfo;
}

// Data Types
export interface Profile {
  id: string;
  name: string;
  description: string;
  scan_type: string;
  ports: string;
  timing: {
    template: string;
  };
  service_detection: boolean;
  os_detection: boolean;
  script_scan: boolean;
  udp_scan: boolean;
  default: boolean;
  usage_count: number;
  created_at: string;
  updated_at: string;
}

export interface Scan {
  id: string;
  name?: string;
  description?: string;
  targets: string[];
  profile_id: string;
  status: string;
  progress: number;
  started_at?: string;
  completed_at?: string;
  duration?: number;
  hosts_discovered: number;
  ports_scanned: number;
  error_message?: string;
  created_at: string;
  updated_at: string;
}

export interface Host {
  id: string;
  ip_address: string;
  hostname?: string;
  mac_address?: string;
  status: string;
  first_seen: string;
  last_seen: string;
  scan_count: number;
  open_ports: number[];
}

export interface DiscoveryJob {
  id: number;
  name: string;
  description?: string;
  networks: string[];
  method: string;
  enabled: boolean;
  status: string;
  progress: number;
  hosts_found: number;
  created_at: string;
  updated_at: string;
}

class ApiService {
  private api: AxiosInstance;

  constructor() {
    this.api = axios.create({
      baseURL: '/api/v1',
      timeout: 10000,
      headers: {
        'Content-Type': 'application/json',
      },
    });
  }

  // Health check
  async getHealth(): Promise<HealthResponse> {
    const response = await this.api.get<HealthResponse>('/health');
    return response.data;
  }

  // Status check
  async getStatus(): Promise<StatusResponse> {
    const response = await this.api.get<StatusResponse>('/status');
    return response.data;
  }

  // Profiles - has actual data
  async getProfiles(): Promise<PaginatedResponse<Profile>> {
    const response = await this.api.get<PaginatedResponse<Profile>>('/profiles');
    return response.data;
  }

  // Scans - empty but working
  async getScans(page = 1, limit = 50): Promise<PaginatedResponse<Scan>> {
    const response = await this.api.get<PaginatedResponse<Scan>>(`/scans?page=${page}&page_size=${limit}`);
    return response.data;
  }

  // Hosts - empty but working
  async getHosts(page = 1, limit = 50): Promise<PaginatedResponse<Host>> {
    const response = await this.api.get<PaginatedResponse<Host>>(`/hosts?page=${page}&page_size=${limit}`);
    return response.data;
  }

  // Discovery - empty but working
  async getDiscoveryJobs(page = 1, limit = 50): Promise<PaginatedResponse<DiscoveryJob>> {
    const response = await this.api.get<PaginatedResponse<DiscoveryJob>>(`/discovery?page=${page}&page_size=${limit}`);
    return response.data;
  }
}

export const apiService = new ApiService();
