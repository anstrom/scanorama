import { useQuery } from "@tanstack/react-query";
import { api } from "../client";

export function useProfile(id: string | undefined) {
  return useQuery({
    queryKey: ["profiles", id],
    queryFn: async () => {
      const { data, error } = await api.GET("/profiles/{profileId}", {
        params: { path: { profileId: id! } },
      });
      if (error) throw error;
      return data;
    },
    enabled: !!id,
  });
}

export function useProfiles(params: { page?: number; page_size?: number } = {}) {
  return useQuery({
    queryKey: ["profiles", params],
    queryFn: async () => {
      const { data, error } = await api.GET("/profiles", {
        params: { query: params },
      });
      if (error) throw error;
      return data;
    },
  });
}
