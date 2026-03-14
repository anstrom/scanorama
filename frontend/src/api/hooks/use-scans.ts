import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../client";

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

export function useCreateScan() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (body: {
      name?: string;
      targets?: string[];
      profile_id?: string;
      description?: string;
    }) => {
      const { data, error } = await api.POST("/scans", { body });
      if (error) throw error;
      return data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["scans"] });
    },
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
