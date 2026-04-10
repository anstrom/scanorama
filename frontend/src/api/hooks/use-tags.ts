import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { ApiError } from "../errors";

const BASE = import.meta.env.VITE_API_BASE_URL ?? "/api/v1";

export function useTags() {
  return useQuery({
    queryKey: ["tags"],
    queryFn: async () => {
      const res = await fetch(`${BASE}/tags`);
      if (!res.ok) {
        const body = await res.json().catch(() => ({}));
        throw new ApiError(res.status, body);
      }
      const data = (await res.json()) as { tags: string[] };
      return data.tags ?? [];
    },
    staleTime: 30_000,
  });
}

export function useUpdateHostTags() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({
      hostId,
      tags,
    }: {
      hostId: string;
      tags: string[];
    }) => {
      const res = await fetch(`${BASE}/hosts/${hostId}/tags`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ tags }),
      });
      if (!res.ok) {
        const body = await res.json().catch(() => ({}));
        throw new ApiError(res.status, body);
      }
    },
    onSuccess: (_data, { hostId }) => {
      void queryClient.invalidateQueries({ queryKey: ["hosts", hostId] });
      void queryClient.invalidateQueries({ queryKey: ["hosts"] });
      void queryClient.invalidateQueries({ queryKey: ["tags"] });
    },
  });
}
