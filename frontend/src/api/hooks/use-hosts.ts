import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../client";
import { ApiError } from "../errors";

interface HostListParams {
  page?: number;
  page_size?: number;
  status?: "up" | "down" | "unknown";
  network?: string;
  search?: string;
  os?: string;
  vendor?: string;
  sort_by?: string;
  sort_order?: "asc" | "desc";
  filter?: string; // structured JSON filter expression (advanced filter)
}

export function useHosts(params: HostListParams = {}) {
  return useQuery({
    queryKey: ["hosts", params],
    queryFn: async () => {
      const { data, error, response } = await api.GET("/hosts", {
        // Cast to any: the generated types are narrower than what the API
        // actually accepts (sort_by, sort_order are not in the spec yet).
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        params: { query: params as any },
      });
      if (error) throw new ApiError(response.status, error);
      return data;
    },
  });
}

export function useHost(id: string) {
  return useQuery({
    queryKey: ["hosts", id],
    queryFn: async () => {
      const { data, error, response } = await api.GET("/hosts/{hostId}", {
        params: { path: { hostId: id } },
      });
      if (error) throw new ApiError(response.status, error);
      return data;
    },
    enabled: !!id,
  });
}

export function useActiveHostCount() {
  return useQuery({
    queryKey: ["hosts", "active-count"],
    queryFn: async () => {
      const { data, error, response } = await api.GET("/hosts", {
        params: { query: { status: "up", page: 1, page_size: 1 } },
      });
      if (error) throw new ApiError(response.status, error);
      return data?.pagination?.total_items ?? 0;
    },
    refetchInterval: 30_000,
  });
}

interface HostScanParams {
  page?: number;
  page_size?: number;
}

export function useHostScans(hostId: string, params: HostScanParams = {}) {
  return useQuery({
    queryKey: ["hosts", hostId, "scans", params],
    queryFn: async () => {
      const { data, error, response } = await api.GET("/hosts/{hostId}/scans", {
        params: { path: { hostId }, query: params },
      });
      if (error) throw new ApiError(response.status, error);
      return data;
    },
    enabled: !!hostId,
  });
}

export function useUpdateHost() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({
      hostId,
      body,
    }: {
      hostId: string;
      body: { hostname?: string; tags?: string[]; notes?: string };
    }) => {
      const { data, error, response } = await api.PUT("/hosts/{hostId}", {
        params: { path: { hostId } },
        body,
      });
      if (error) throw new ApiError(response.status, error);
      return data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["hosts"] });
    },
  });
}

export function useDeleteHost() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (hostId: string) => {
      const { error, response } = await api.DELETE("/hosts/{hostId}", {
        params: { path: { hostId } },
      });
      if (error) throw new ApiError(response.status, error);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["hosts"] });
    },
  });
}

// useUpdateCustomName sets or clears the user-defined display-name override
// for a host via PATCH /hosts/{hostId}/custom-name. Pass null in the body to
// clear the override; the backend also treats whitespace-only strings as a
// clear. Invalidates the ["hosts"] cache so the list + detail views both pick
// up the new display_name on next render.
export function useUpdateCustomName() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({
      hostId,
      customName,
    }: {
      hostId: string;
      customName: string | null;
    }) => {
      // openapi-fetch exposes PATCH but the generated types haven't picked up
      // the new endpoint on this build in every environment. Cast narrowly so
      // we stay type-safe on hostId while tolerating a brief type lag.
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      const { data, error, response } = await (api as any).PATCH(
        "/hosts/{hostId}/custom-name",
        {
          params: { path: { hostId } },
          body: { custom_name: customName },
        },
      );
      if (error) throw new ApiError(response.status, error);
      return data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["hosts"] });
    },
  });
}

// useRefreshIdentity enqueues an identity_enrichment scan for the host via
// POST /smart-scan/hosts/{hostId}/refresh-identity. The backend runs this
// unconditionally — the button exists for users who want to poke a host
// right now, not for the orchestrator's decision logic. Returns the scan id
// so callers can poll job status if they want.
export function useRefreshIdentity() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (hostId: string) => {
      const { data, error, response } = await api.POST(
        "/smart-scan/hosts/{id}/refresh-identity",
        { params: { path: { id: hostId } } },
      );
      if (error) throw new ApiError(response.status, error);
      return data;
    },
    onSuccess: (_data, hostId) => {
      // Name candidates live on the host detail response; invalidate so the
      // Identity tab refetches after the scan lands.
      queryClient.invalidateQueries({ queryKey: ["hosts", hostId] });
    },
  });
}

export function useBulkDeleteHosts() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (ids: string[]) => {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      const { data, error, response } = await (api as any).DELETE("/hosts", {
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        body: { ids } as any,
      });
      if (error) throw new ApiError(response.status, error);
      return data as { deleted: number } | undefined;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["hosts"] });
    },
  });
}
