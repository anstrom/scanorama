import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../client";
import { ApiError } from "../errors";
import type { components } from "../types";

export type ScanStage = components["schemas"]["docs.ScanStageResponse"];
export type SuggestionSummary =
  components["schemas"]["docs.SuggestionSummaryResponse"];
export type BatchResult = components["schemas"]["docs.BatchResultResponse"];

export function useSmartScanStage(hostId: string) {
  return useQuery({
    queryKey: ["smart-scan", "stage", hostId],
    queryFn: async () => {
      const { data, error, response } = await api.GET(
        "/smart-scan/hosts/{id}/stage",
        { params: { path: { id: hostId } } },
      );
      if (error) throw new ApiError(response.status, error);
      return data as ScanStage;
    },
    enabled: !!hostId,
    staleTime: 30_000,
  });
}

export function useSmartScanSuggestions(enabled = true) {
  return useQuery({
    queryKey: ["smart-scan", "suggestions"],
    queryFn: async () => {
      const { data, error, response } = await api.GET(
        "/smart-scan/suggestions",
      );
      if (error) throw new ApiError(response.status, error);
      return data as SuggestionSummary;
    },
    enabled,
    staleTime: 60_000,
  });
}

export type TriggerHostResponse = {
  host_id?: string;
  queued?: boolean;
  scan_id?: string;
  message?: string;
};

export function useTriggerSmartScan() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (hostId: string) => {
      const { data, error, response } = await api.POST(
        "/smart-scan/hosts/{id}/trigger",
        { params: { path: { id: hostId } } },
      );
      if (error) throw new ApiError(response.status, error);
      return data as TriggerHostResponse;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["scans"] });
      queryClient.invalidateQueries({ queryKey: ["smart-scan"] });
    },
  });
}

export type ProfileRecommendation = {
  os_family: string;
  host_count: number;
  profile_id: string;
  profile_name: string;
  action: string;
};

export function useProfileRecommendations(enabled = true) {
  return useQuery({
    queryKey: ["smart-scan", "profile-recommendations"],
    queryFn: async () => {
      const { data, error, response } = await api.GET(
        "/smart-scan/profile-recommendations" as never,
        {} as never,
      );
      if (error) throw new ApiError(response.status, error);
      return (data as ProfileRecommendation[]) ?? [];
    },
    enabled,
    staleTime: 120_000,
  });
}

export function useTriggerSmartScanBatch() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (
      body: components["schemas"]["docs.TriggerBatchRequest"],
    ) => {
      const { data, error, response } = await api.POST(
        "/smart-scan/trigger-batch",
        { body },
      );
      if (error) throw new ApiError(response.status, error);
      return data as BatchResult;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["scans"] });
      queryClient.invalidateQueries({ queryKey: ["smart-scan"] });
    },
  });
}
