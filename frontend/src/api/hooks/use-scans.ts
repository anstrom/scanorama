import { useMemo } from "react";
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
  sort_by?: string;
  sort_order?: "asc" | "desc";
}

export function useScans(params: ScanListParams = {}) {
  return useQuery({
    queryKey: ["scans", "list", params],
    queryFn: async () => {
      const { data, error, response } = await api.GET("/scans", {
        // Cast to any: the generated types are narrower than what the API
        // actually accepts (sort_by, sort_order, stopped status are not in the spec yet).
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        params: { query: params as any },
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

export function useStopScan() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (scanId: string) => {
      const { data, error, response } = await api.POST("/scans/{scanId}/stop", {
        params: { path: { scanId } },
      });
      if (error) throw new ApiError(response.status, error);
      return data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["scans"] });
    },
  });
}

export function useDeleteScan() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (scanId: string) => {
      const { error, response } = await api.DELETE("/scans/{scanId}", {
        params: { path: { scanId } },
      });
      if (error) throw new ApiError(response.status, error);
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

export interface ScanActivityDay {
  date: string;
  completed: number;
  failed: number;
  running: number;
}

export function useScanActivity(): {
  data: ScanActivityDay[];
  isLoading: boolean;
} {
  const { data, isLoading } = useQuery({
    queryKey: ["scans", "activity"],
    queryFn: async () => {
      const { data, error, response } = await api.GET("/scans", {
        params: { query: { page: 1, page_size: 200 } },
      });
      if (error) throw new ApiError(response.status, error);
      return data;
    },
    staleTime: 5 * 60 * 1000,
  });

  const activity: ScanActivityDay[] = useMemo(() => {
    const scans = data?.data ?? [];
    const days: ScanActivityDay[] = [];
    const now = new Date();

    for (let i = 6; i >= 0; i--) {
      const d = new Date(now);
      d.setDate(d.getDate() - i);
      const dayStr = d.toISOString().slice(0, 10); // YYYY-MM-DD

      const label =
        i === 0 ? "Today" : d.toLocaleDateString("en-US", { weekday: "short" });

      const dayScans = scans.filter((s) => {
        const ts = s.started_at ?? s.created_at ?? "";
        return ts.slice(0, 10) === dayStr;
      });

      days.push({
        date: label,
        completed: dayScans.filter((s) => s.status === "completed").length,
        failed: dayScans.filter((s) => s.status === "failed").length,
        running: dayScans.filter(
          (s) => s.status === "running" || s.status === "pending",
        ).length,
      });
    }
    return days;
  }, [data]);

  return { data: activity, isLoading };
}
