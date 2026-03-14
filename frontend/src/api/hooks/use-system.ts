import { useQuery } from "@tanstack/react-query";
import { api } from "../client";

export function useHealth() {
  return useQuery({
    queryKey: ["health"],
    queryFn: async () => {
      const { data, error } = await api.GET("/health");
      if (error) throw error;
      return data;
    },
    refetchInterval: 30_000,
  });
}

export function useStatus() {
  return useQuery({
    queryKey: ["status"],
    queryFn: async () => {
      const { data, error } = await api.GET("/status");
      if (error) throw error;
      return data;
    },
    refetchInterval: 30_000,
  });
}

export function useVersion() {
  return useQuery({
    queryKey: ["version"],
    queryFn: async () => {
      const { data, error } = await api.GET("/version");
      if (error) throw error;
      return data;
    },
    staleTime: Infinity,
  });
}
