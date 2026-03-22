import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../client";
import type { components } from "../types";

type CreateScheduleRequest = components["schemas"]["docs.CreateScheduleRequest"];

interface ScheduleListParams {
  page?: number;
  page_size?: number;
  enabled?: boolean;
}

export function useSchedules(params: ScheduleListParams = {}) {
  return useQuery({
    queryKey: ["schedules", params],
    queryFn: async () => {
      const { data, error } = await api.GET("/schedules", {
        params: { query: params },
      });
      if (error) throw error;
      return data;
    },
  });
}

export function useSchedule(id: string) {
  return useQuery({
    queryKey: ["schedules", id],
    queryFn: async () => {
      const { data, error } = await api.GET("/schedules/{scheduleId}", {
        params: { path: { scheduleId: id } },
      });
      if (error) throw error;
      return data;
    },
    enabled: !!id,
  });
}

export function useCreateSchedule() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (body: CreateScheduleRequest) => {
      const { data, error } = await api.POST("/schedules", { body });
      if (error) {
        const apiError = error as { message?: string; error?: string };
        throw new Error(
          apiError.message ?? apiError.error ?? "Failed to create schedule.",
        );
      }
      return data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["schedules"] });
    },
  });
}

export function useUpdateSchedule() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ id, body }: { id: string; body: CreateScheduleRequest }) => {
      const { data, error } = await api.PUT("/schedules/{scheduleId}", {
        params: { path: { scheduleId: id } },
        body,
      });
      if (error) {
        const apiError = error as { message?: string; error?: string };
        throw new Error(
          apiError.message ?? apiError.error ?? "Failed to update schedule.",
        );
      }
      return data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["schedules"] });
    },
  });
}

export function useDeleteSchedule() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (scheduleId: string) => {
      const { error } = await api.DELETE("/schedules/{scheduleId}", {
        params: { path: { scheduleId } },
      });
      if (error) {
        const apiError = error as { message?: string; error?: string };
        throw new Error(
          apiError.message ?? apiError.error ?? "Failed to delete schedule.",
        );
      }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["schedules"] });
    },
  });
}

export function useEnableSchedule() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (scheduleId: string) => {
      const { data, error } = await api.POST("/schedules/{scheduleId}/enable", {
        params: { path: { scheduleId } },
      });
      if (error) {
        const apiError = error as { message?: string; error?: string };
        throw new Error(
          apiError.message ?? apiError.error ?? "Failed to enable schedule.",
        );
      }
      return data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["schedules"] });
    },
  });
}

export function useDisableSchedule() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (scheduleId: string) => {
      const { data, error } = await api.POST("/schedules/{scheduleId}/disable", {
        params: { path: { scheduleId } },
      });
      if (error) {
        const apiError = error as { message?: string; error?: string };
        throw new Error(
          apiError.message ?? apiError.error ?? "Failed to disable schedule.",
        );
      }
      return data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["schedules"] });
    },
  });
}
