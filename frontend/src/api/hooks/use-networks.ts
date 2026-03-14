import { useQuery } from "@tanstack/react-query";
import { api } from "../client";

interface NetworkListParams {
  page?: number;
  page_size?: number;
}

export function useNetworks(params: NetworkListParams = {}) {
  return useQuery({
    queryKey: ["networks", params],
    queryFn: async () => {
      const { data, error } = await api.GET("/networks", {
        params: { query: params },
      });
      if (error) throw error;
      return data;
    },
  });
}

export function useNetworkStats() {
  return useQuery({
    queryKey: ["networks", "stats"],
    queryFn: async () => {
      const { data, error } = await api.GET("/networks/stats");
      if (error) throw error;
      return data;
    },
    refetchInterval: 30_000,
  });
}
