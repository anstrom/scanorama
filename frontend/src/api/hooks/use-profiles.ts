import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../client";
import { ApiError } from "../errors";
import type { components } from "../types";

type CreateProfileRequest = components["schemas"]["docs.CreateProfileRequest"];

export function useProfile(id: string | undefined) {
  return useQuery({
    queryKey: ["profiles", id],
    queryFn: async () => {
      const { data, error, response } = await api.GET("/profiles/{profileId}", {
        params: { path: { profileId: id! } },
      });
      if (error) throw new ApiError(response.status, error);
      return data;
    },
    enabled: !!id,
  });
}

export function useProfiles(
  params: {
    page?: number;
    page_size?: number;
    sort_by?: string;
    sort_order?: "asc" | "desc";
  } = {},
) {
  return useQuery({
    queryKey: ["profiles", params],
    queryFn: async () => {
      const { data, error, response } = await api.GET("/profiles", {
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

export function useCreateProfile() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (body: CreateProfileRequest) => {
      const { data, error, response } = await api.POST("/profiles", { body });
      if (error) throw new ApiError(response.status, error);
      return data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["profiles"] });
    },
  });
}

export function useUpdateProfile() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({
      id,
      body,
    }: {
      id: string;
      body: CreateProfileRequest;
    }) => {
      const { data, error, response } = await api.PUT("/profiles/{profileId}", {
        params: { path: { profileId: id } },
        body,
      });
      if (error) throw new ApiError(response.status, error);
      return data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["profiles"] });
    },
  });
}

export function useDeleteProfile() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (profileId: string) => {
      const { error, response } = await api.DELETE("/profiles/{profileId}", {
        params: { path: { profileId } },
      });
      if (error) throw new ApiError(response.status, error);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["profiles"] });
    },
  });
}
