import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../client";
import type { components } from "../types";

type CreateNetworkRequest = components["schemas"]["docs.CreateNetworkRequest"];
type UpdateNetworkRequest = components["schemas"]["docs.UpdateNetworkRequest"];
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
      body: UpdateNetworkRequest;
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

export function useStartNetworkDiscovery() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (networkId: string) => {
      const { data, error } = await api.POST("/networks/{networkId}/discover", {
        params: { path: { networkId } },
      });
      if (error) {
        const apiError = error as { message?: string; error?: string };
        throw new Error(
          apiError.message ??
            apiError.error ??
            "Failed to start network discovery.",
        );
      }
      return data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["discovery"] });
      queryClient.invalidateQueries({ queryKey: ["networks"] });
    },
  });
}

export function useNetworkDiscoveryJobs(
  networkId: string,
  params: { page?: number; page_size?: number } = {},
) {
  return useQuery({
    queryKey: ["networks", networkId, "discovery", params],
    queryFn: async () => {
      const base =
        (import.meta.env.VITE_API_BASE_URL as string | undefined) ?? "/api/v1";
      const url = new URL(
        `${base}/networks/${networkId}/discovery`,
        window.location.origin,
      );
      if (params.page != null)
        url.searchParams.set("page", String(params.page));
      if (params.page_size != null)
        url.searchParams.set("page_size", String(params.page_size));
      const resp = await fetch(url.toString());
      if (!resp.ok) throw new Error("Failed to fetch network discovery jobs");
      return resp.json() as Promise<{
        data?: components["schemas"]["docs.DiscoveryJobResponse"][];
        pagination?: components["schemas"]["docs.PaginationInfo"];
      }>;
    },
    enabled: !!networkId,
    refetchInterval: 10_000,
  });
}

export function useStartNetworkScan() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({
      networkId,
      osDetection = false,
    }: {
      networkId: string;
      osDetection?: boolean;
    }) => {
      const base =
        (import.meta.env.VITE_API_BASE_URL as string | undefined) ?? "/api/v1";
      const resp = await fetch(`${base}/networks/${networkId}/scan`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ os_detection: osDetection }),
      });
      if (!resp.ok) {
        const body = (await resp.json().catch(() => ({}))) as {
          message?: string;
          error?: string;
        };
        throw new Error(
          body.message ?? body.error ?? "Failed to create network scan",
        );
      }
      return resp.json() as Promise<{
        id: string;
        name: string;
        targets: string[];
        status: string;
      }>;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["scans"] });
    },
  });
}
