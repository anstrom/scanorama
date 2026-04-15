import { useQuery } from "@tanstack/react-query";
import { api } from "../client";
import { ApiError } from "../errors";

export function useHostNetworks(id: string) {
  return useQuery({
    queryKey: ["hosts", id, "networks"],
    queryFn: async () => {
      const { data, error, response } = await api.GET(
        "/hosts/{hostId}/networks",
        { params: { path: { hostId: id } } },
      );
      if (error) throw new ApiError(response.status, error);
      return data ?? [];
    },
    enabled: !!id,
    staleTime: 30_000,
  });
}
