import { useQuery } from "@tanstack/react-query";
import { api } from "../client";
import type { components } from "../types";

type BaseVersionResponse = components["schemas"]["docs.VersionResponse"];

export type VersionInfo = BaseVersionResponse & {
  commit?: string;
  build_time?: string;
};

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
      return data as VersionInfo | undefined;
    },
    staleTime: Infinity,
  });
}

export function useAdminStatus() {
  return useQuery({
    queryKey: ["admin", "status"],
    queryFn: async () => {
      const { data, error } = await api.GET("/admin/status");
      if (error) throw error;
      return data;
    },
    refetchInterval: 30_000,
  });
}

export function useWorkers() {
  return useQuery({
    queryKey: ["admin", "workers"],
    queryFn: async () => {
      // /admin/workers is not yet reflected in the generated OpenAPI types;
      // cast to any to bypass the path-level type check until types are regenerated.
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      const { data, error } = await (api as any).GET("/admin/workers");
      if (error) throw error;
      return data;
    },
    refetchInterval: 10_000,
  });
}
