import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../client";
import type { components } from "../types";

type CreateDiscoveryJobRequest =
  components["schemas"]["docs.CreateDiscoveryJobRequest"];

interface DiscoveryListParams {
  page?: number;
  page_size?: number;
}

// ── Queries ──────────────────────────────────────────────────────────────────

export function useDiscoveryJobs(params: DiscoveryListParams = {}) {
  return useQuery({
    queryKey: ["discovery", params],
    queryFn: async () => {
      const { data, error } = await api.GET("/discovery", {
        params: { query: params },
      });
      if (error) throw error;
      return data;
    },
    refetchInterval: (query) => {
      const jobs = query.state.data?.data ?? [];
      const hasActive = jobs.some(
        (j) => j.status === "pending" || j.status === "running",
      );
      return hasActive ? 3_000 : false;
    },
  });
}

export function useDiscoveryJob(id: string) {
  return useQuery({
    queryKey: ["discovery", id],
    queryFn: async () => {
      const { data, error } = await api.GET("/discovery/{discoveryId}", {
        params: { path: { discoveryId: id } },
      });
      if (error) throw error;
      return data;
    },
    enabled: !!id,
  });
}

// ── Mutations ─────────────────────────────────────────────────────────────────

export function useCreateDiscoveryJob() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (body: CreateDiscoveryJobRequest) => {
      const { data, error } = await api.POST("/discovery", { body });
      if (error) {
        const apiError = error as { message?: string; error?: string };
        throw new Error(
          apiError.message ??
            apiError.error ??
            "Failed to create discovery job.",
        );
      }
      return data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["discovery"] });
    },
  });
}

export function useStartDiscovery() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (discoveryId: string) => {
      const { data, error } = await api.POST("/discovery/{discoveryId}/start", {
        params: { path: { discoveryId } },
      });
      if (error) {
        const apiError = error as { message?: string; error?: string };
        throw new Error(
          apiError.message ??
            apiError.error ??
            "Failed to start discovery job.",
        );
      }
      return data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["discovery"] });
    },
  });
}

export function useStopDiscovery() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (discoveryId: string) => {
      const { data, error } = await api.POST("/discovery/{discoveryId}/stop", {
        params: { path: { discoveryId } },
      });
      if (error) {
        const apiError = error as { message?: string; error?: string };
        throw new Error(
          apiError.message ?? apiError.error ?? "Failed to stop discovery job.",
        );
      }
      return data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["discovery"] });
    },
  });
}
