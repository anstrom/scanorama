import { useQuery, keepPreviousData } from '@tanstack/react-query';
import {
  apiService,
  HealthResponse,
  StatusResponse,
  PaginatedResponse,
  Profile,
  Scan,
  Host,
  DiscoveryJob
} from '../services/api';

// Health check hook
export function useHealth() {
  return useQuery<HealthResponse, Error>({
    queryKey: ['health'],
    queryFn: () => apiService.getHealth(),
    refetchInterval: 30000, // Refresh every 30 seconds
    retry: 2,
  });
}

// Status check hook
export function useStatus() {
  return useQuery<StatusResponse, Error>({
    queryKey: ['status'],
    queryFn: () => apiService.getStatus(),
    refetchInterval: 60000, // Refresh every minute
    retry: 2,
  });
}

// Profiles hook - has actual data
export function useProfiles() {
  return useQuery<PaginatedResponse<Profile>, Error>({
    queryKey: ['profiles'],
    queryFn: () => apiService.getProfiles(),
    staleTime: 5 * 60 * 1000, // Profiles don't change often
  });
}

// Scans hook - currently empty but endpoint works
export function useScans(page = 1, limit = 50) {
  return useQuery<PaginatedResponse<Scan>, Error>({
    queryKey: ['scans', page, limit],
    queryFn: () => apiService.getScans(page, limit),
    refetchInterval: 10000, // Refresh every 10 seconds
    placeholderData: keepPreviousData,
  });
}

// Hosts hook - currently empty but endpoint works
export function useHosts(page = 1, limit = 50) {
  return useQuery<PaginatedResponse<Host>, Error>({
    queryKey: ['hosts', page, limit],
    queryFn: () => apiService.getHosts(page, limit),
    refetchInterval: 30000, // Refresh every 30 seconds
    placeholderData: keepPreviousData,
  });
}

// Discovery Jobs hook - currently empty but endpoint works
export function useDiscoveryJobs(page = 1, limit = 50) {
  return useQuery<PaginatedResponse<DiscoveryJob>, Error>({
    queryKey: ['discovery', page, limit],
    queryFn: () => apiService.getDiscoveryJobs(page, limit),
    refetchInterval: 15000, // Refresh every 15 seconds
    placeholderData: keepPreviousData,
  });
}
