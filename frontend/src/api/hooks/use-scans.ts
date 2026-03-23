import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../client";
import { ApiError } from "../errors";

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

type ScanStatus =
  | "pending"
  | "running"
  | "completed"
  | "failed"
  | "cancelled"
  | "stopped";

interface ScanListParams {
  page?: number;
  page_size?: number;
  status?: ScanStatus;
}

export function useScans(params: ScanListParams = {}) {
  return useQuery({
    queryKey: ["scans", "list", params],
    queryFn: async () => {
      // Cast status: the generated types omit "stopped" but the backend supports it.
      const query = params as Omit<typeof params, "status"> & {
        status?: "pending" | "running" | "completed" | "failed" | "cancelled";
      };
      const { data, error, response } = await api.GET("/scans", {
        params: { query },
      });
      if (error) throw new ApiError(response.status, error);
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
    queryKey: ["scans", "detail", id],
    queryFn: async () => {
      const { data, error, response } = await api.GET("/scans/{scanId}", {
        params: { path: { scanId: id } },
      });
      if (error) throw new ApiError(response.status, error);
      return data;
    },
    enabled: !!id,
  });
}

export function useStartScan() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (scanId: string) => {
      const { data, error, response } = await api.POST(
        "/scans/{scanId}/start",
        {
          params: { path: { scanId } },
        },
      );
      if (error) throw new ApiError(response.status, error);
      return data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["scans"] });
    },
  });
}

export type ScanType =
  | "connect"
  | "syn"
  | "ack"
  | "udp"
  | "aggressive"
  | "comprehensive";

export function useCreateScan() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (body: {
      name: string;
      targets: string[];
      scan_type: ScanType;
      ports?: string;
      os_detection?: boolean;
      description?: string;
    }) => {
      const { data, error, response } = await api.POST("/scans", { body });
      if (error) throw new ApiError(response.status, error);
      return data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["scans"] });
    },
  });
}

export function useScanResults(scanId: string, scanStatus?: string) {
  return useQuery({
    queryKey: ["scans", "results", scanId],
    queryFn: async () => {
      const { data, error, response } = await api.GET(
        "/scans/{scanId}/results",
        {
          params: { path: { scanId } },
        },
      );
      if (error) throw new ApiError(response.status, error);
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
      const { data, error, response } = await api.GET("/scans", {
        params: { query: { page: 1, page_size: limit } },
      });
      if (error) throw new ApiError(response.status, error);
      return data;
    },
    refetchInterval: 30_000,
  });
}
