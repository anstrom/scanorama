import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../client";
import { ApiError } from "../errors";

export function useDevices() {
  return useQuery({
    queryKey: ["devices"],
    queryFn: async () => {
      const { data, error, response } = await api.GET("/devices");
      if (error) throw new ApiError(response.status, error);
      return data?.devices ?? [];
    },
  });
}

export function useDevice(id: string) {
  return useQuery({
    queryKey: ["devices", id],
    queryFn: async () => {
      const { data, error, response } = await api.GET("/devices/{id}", {
        params: { path: { id } },
      });
      if (error) throw new ApiError(response.status, error);
      return data;
    },
    enabled: !!id,
  });
}

export function useCreateDevice() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (body: { name: string; notes?: string }) => {
      const { data, error, response } = await api.POST("/devices", { body });
      if (error) throw new ApiError(response.status, error);
      return data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["devices"] });
    },
  });
}

export function useUpdateDevice() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ id, body }: { id: string; body: { name?: string; notes?: string } }) => {
      const { data, error, response } = await api.PUT("/devices/{id}", {
        params: { path: { id } },
        body,
      });
      if (error) throw new ApiError(response.status, error);
      return data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["devices"] });
    },
  });
}

export function useDeleteDevice() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (id: string) => {
      const { error, response } = await api.DELETE("/devices/{id}", {
        params: { path: { id } },
      });
      if (error) throw new ApiError(response.status, error);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["devices"] });
    },
  });
}

export function useAttachHost() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ deviceId, hostId }: { deviceId: string; hostId: string }) => {
      const { error, response } = await api.POST("/devices/{id}/hosts/{host_id}", {
        params: { path: { id: deviceId, host_id: hostId } },
      });
      if (error) throw new ApiError(response.status, error);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["devices"] });
      queryClient.invalidateQueries({ queryKey: ["hosts"] });
    },
  });
}

export function useDetachHost() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ deviceId, hostId }: { deviceId: string; hostId: string }) => {
      const { error, response } = await api.DELETE("/devices/{id}/hosts/{host_id}", {
        params: { path: { id: deviceId, host_id: hostId } },
      });
      if (error) throw new ApiError(response.status, error);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["devices"] });
      queryClient.invalidateQueries({ queryKey: ["hosts"] });
    },
  });
}

export function useAcceptSuggestion() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (id: string) => {
      const { error, response } = await api.POST("/devices/suggestions/{id}/accept", {
        params: { path: { id } },
      });
      if (error) throw new ApiError(response.status, error);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["devices"] });
      queryClient.invalidateQueries({ queryKey: ["hosts"] });
      queryClient.invalidateQueries({ queryKey: ["discovery"] });
    },
  });
}

export function useDismissSuggestion() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (id: string) => {
      const { error, response } = await api.POST("/devices/suggestions/{id}/dismiss", {
        params: { path: { id } },
      });
      if (error) throw new ApiError(response.status, error);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["discovery"] });
    },
  });
}
