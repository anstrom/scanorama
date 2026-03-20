import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../client";

export interface ScanResultEntry {
  id?: string;
  host_ip?: string;
  hostname?: string;
  port?: number;
  protocol?: string;
  state?: string;
  service?: string;
  version?: string;
  banner?: string;
  scan_time?: string;
  os_name?: string;
  os_family?: string;
  os_version?: string;
  os_confidence?: number;
}

export interface ScanResultsSummary {
  scan_id?: string;
  total_hosts?: number;
  total_ports?: number;
  open_ports?: number;
  closed_ports?: number;
  duration?: string;
}

export interface ScanResultsData {
  scan_id?: string;
  total_hosts?: number;
  total_ports?: number;
  open_ports?: number;
  closed_ports?: number;
  generated_at?: string;
  results?: ScanResultEntry[];
  summary?: ScanResultsSummary;
}

interface ScanListParams {
  page?: number;
  page_size?: number;
  status?: "pending" | "running" | "completed" | "failed" | "cancelled";
}

export function useScans(params: ScanListParams = {}) {
  return useQuery({
    queryKey: ["scans", params],
    queryFn: async () => {
      const { data, error } = await api.GET("/scans", {
        params: { query: params },
      });
      if (error) throw error;
      return data;
    },
    refetchInterval: (query) => {
      const scans = query.state.data?.data ?? [];
      const hasActive = scans.some(
        (s) => s.status === "pending" || s.status === "running",
      );
      return hasActive ? 3_000 : false;
    },
  });
}

export function useScan(id: string) {
  return useQuery({
    queryKey: ["scans", id],
    queryFn: async () => {
      const { data, error } = await api.GET("/scans/{scanId}", {
        params: { path: { scanId: id } },
      });
      if (error) throw error;
      return data;
    },
    enabled: !!id,
  });
}

export function useStartScan() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (scanId: string) => {
      const { data, error } = await api.POST("/scans/{scanId}/start", {
        params: { path: { scanId } },
      });
      if (error) throw error;
      return data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["scans"] });
    },
  });
}

export function useCreateScan() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (body: {
      name: string;
      targets: string[];
      scan_type: string;
      ports?: string;
      os_detection?: boolean;
      description?: string;
    }) => {
      const { data, error } = await api.POST("/scans", { body });
      if (error) {
        throw new Error(
          typeof (error as { message?: string }).message === "string"
            ? (error as { message: string }).message
            : "Scan creation failed.",
        );
      }
      return data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["scans"] });
    },
  });
}

export function useScanResults(
  scanId: string,
  _params?: { page?: number; page_size?: number },
  scanStatus?: string,
) {
  return useQuery({
    queryKey: ["scans", scanId, "results"],
    queryFn: async () => {
      const { data, error } = await api.GET("/scans/{scanId}/results", {
        params: { path: { scanId } },
      });
      if (error) throw error;
      return data as ScanResultsData | undefined;
    },
    enabled: !!scanId,
    refetchInterval:
      scanStatus === "pending" || scanStatus === "running" ? 3_000 : false,
  });
}

export function useRecentScans(limit = 5) {
  return useQuery({
    queryKey: ["scans", "recent", limit],
    queryFn: async () => {
      const { data, error } = await api.GET("/scans", {
        params: { query: { page: 1, page_size: limit } },
      });
      if (error) throw error;
      return data;
    },
    refetchInterval: 30_000,
  });
}
