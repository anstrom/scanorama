import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../client";
import type { components } from "../types";

type CreateNetworkRequest = components["schemas"]["docs.CreateNetworkRequest"];
type CreateExclusionRequest =
  components["schemas"]["docs.CreateExclusionRequest"];

interface NetworkListParams {
  page?: number;
  page_size?: number;
  show_inactive?: boolean;
  name?: string;
}

// ── Queries ─────────────────────────────────────────────────────────────────────────────────────

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

export function useNetwork(id: string) {
  return useQuery({
    queryKey: ["networks", id],
    queryFn: async () => {
      const { data, error } = await api.GET("/networks/{networkId}", {
        params: { path: { networkId: id } },
      });
      if (error) throw error;
      return data;
    },
    enabled: !!id,
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

export function useNetworkExclusions(networkId: string) {
  return useQuery({
    queryKey: ["networks", networkId, "exclusions"],
    queryFn: async () => {
      const { data, error } = await api.GET(
        "/networks/{networkId}/exclusions",
        {
          params: { path: { networkId } },
        },
      );
      if (error) throw error;
      return data;
    },
    enabled: !!networkId,
  });
}

export function useGlobalExclusions() {
  return useQuery({
    queryKey: ["exclusions", "global"],
    queryFn: async () => {
      const { data, error } = await api.GET("/exclusions");
      if (error) throw error;
      return data;
    },
  });
}

// ── Mutations ─────────────────────────────────────────────────────────────────────────────────

export function useCreateNetwork() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (body: CreateNetworkRequest) => {
      const { data, error } = await api.POST("/networks", { body });
      if (error) {
        const apiError = error as { message?: string; error?: string };
        throw new Error(
          apiError.message ?? apiError.error ?? "Failed to create network.",
        );
      }
      return data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["networks"] });
    },
  });
}

export function useDeleteNetwork() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (networkId: string) => {
      const { error } = await api.DELETE("/networks/{networkId}", {
        params: { path: { networkId } },
      });
      if (error) {
        const apiError = error as { message?: string; error?: string };
        throw new Error(
          apiError.message ?? apiError.error ?? "Failed to delete network.",
        );
      }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["networks"] });
    },
  });
}

export function useEnableNetwork() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (networkId: string) => {
      const { data, error } = await api.POST("/networks/{networkId}/enable", {
        params: { path: { networkId } },
      });
      if (error) {
        const apiError = error as { message?: string; error?: string };
        throw new Error(
          apiError.message ?? apiError.error ?? "Failed to enable network.",
        );
      }
      return data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["networks"] });
    },
  });
}

export function useDisableNetwork() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (networkId: string) => {
      const { data, error } = await api.POST("/networks/{networkId}/disable", {
        params: { path: { networkId } },
      });
      if (error) {
        const apiError = error as { message?: string; error?: string };
        throw new Error(
          apiError.message ?? apiError.error ?? "Failed to disable network.",
        );
      }
      return data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["networks"] });
    },
  });
}

export function useRenameNetwork() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({
      networkId,
      newName,
    }: {
      networkId: string;
      newName: string;
    }) => {
      const { data, error } = await api.PUT("/networks/{networkId}/rename", {
        params: { path: { networkId } },
        body: { new_name: newName },
      });
      if (error) {
        const apiError = error as { message?: string; error?: string };
        throw new Error(
          apiError.message ?? apiError.error ?? "Failed to rename network.",
        );
      }
      return data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["networks"] });
    },
  });
}

export function useCreateNetworkExclusion() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({
      networkId,
      body,
    }: {
      networkId: string;
      body: CreateExclusionRequest;
    }) => {
      const { data, error } = await api.POST(
        "/networks/{networkId}/exclusions",
        {
          params: { path: { networkId } },
          body,
        },
      );
      if (error) {
        const apiError = error as { message?: string; error?: string };
        throw new Error(
          apiError.message ?? apiError.error ?? "Failed to add exclusion.",
        );
      }
      return data;
    },
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: ["networks", variables.networkId, "exclusions"],
      });
    },
  });
}

export function useCreateGlobalExclusion() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (body: CreateExclusionRequest) => {
      const { data, error } = await api.POST("/exclusions", { body });
      if (error) {
        const apiError = error as { message?: string; error?: string };
        throw new Error(
          apiError.message ?? apiError.error ?? "Failed to add exclusion.",
        );
      }
      return data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["exclusions"] });
    },
  });
}

export function useUpdateNetwork() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({
      networkId,
      body,
    }: {
      networkId: string;
      body: CreateNetworkRequest;
    }) => {
      const { data, error } = await api.PUT("/networks/{networkId}", {
        params: { path: { networkId } },
        body,
      });
      if (error) {
        const apiError = error as { message?: string; error?: string };
        throw new Error(
          apiError.message ?? apiError.error ?? "Failed to update network.",
        );
      }
      return data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["networks"] });
    },
  });
}

export function useDeleteExclusion() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (exclusionId: string) => {
      const { error } = await api.DELETE("/exclusions/{exclusionId}", {
        params: { path: { exclusionId } },
      });
      if (error) {
        const apiError = error as { message?: string; error?: string };
        throw new Error(
          apiError.message ?? apiError.error ?? "Failed to delete exclusion.",
        );
      }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["networks"] });
      queryClient.invalidateQueries({ queryKey: ["exclusions"] });
    },
  });
}
