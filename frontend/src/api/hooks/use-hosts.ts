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
