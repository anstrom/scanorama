import { useQuery } from "@tanstack/react-query";
import { api } from "../client";

interface HostListParams {
  page?: number;
  page_size?: number;
  status?: "up" | "down" | "unknown";
  search?: string;
}

export function useHosts(params: HostListParams = {}) {
  return useQuery({
    queryKey: ["hosts", params],
    queryFn: async () => {
      const { data, error } = await api.GET("/hosts", {
        params: { query: params },
      });
      if (error) throw error;
      return data;
    },
  });
}

export function useHost(id: string) {
  return useQuery({
    queryKey: ["hosts", id],
    queryFn: async () => {
      const { data, error } = await api.GET("/hosts/{hostId}", {
        params: { path: { hostId: id } },
      });
      if (error) throw error;
      return data;
    },
    enabled: !!id,
  });
}

export function useActiveHostCount() {
  return useQuery({
    queryKey: ["hosts", "active-count"],
    queryFn: async () => {
      const { data, error } = await api.GET("/hosts", {
        params: { query: { status: "up", page: 1, page_size: 1 } },
      });
      if (error) throw error;
      return data?.pagination?.total_items ?? 0;
    },
    refetchInterval: 30_000,
  });
}
