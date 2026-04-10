import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { ApiError } from "../errors";

const BASE = import.meta.env.VITE_API_BASE_URL ?? "/api/v1";

export interface HostGroup {
  id: string;
  name: string;
  description?: string;
  color?: string;
  member_count: number;
  created_at: string;
  updated_at: string;
}

export interface GroupMember {
  id: string;
  ip_address: string;
  hostname?: string;
  status: string;
  tags?: string[];
  last_seen: string;
}

async function apiFetch<T>(
  path: string,
  init?: RequestInit,
): Promise<T> {
  const res = await fetch(`${BASE}${path}`, init);
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new ApiError(res.status, body);
  }
  if (res.status === 204) return undefined as T;
  return res.json() as Promise<T>;
}

// ── Queries ───────────────────────────────────────────────────────────────────

export function useGroups() {
  return useQuery({
    queryKey: ["groups"],
    queryFn: () =>
      apiFetch<{ groups: HostGroup[]; total: number }>("/groups").then(
        (d) => d.groups,
      ),
  });
}

export function useGroup(id: string | undefined) {
  return useQuery({
    queryKey: ["groups", id],
    queryFn: () => apiFetch<HostGroup>(`/groups/${id!}`),
    enabled: !!id,
  });
}

export function useGroupMembers(
  id: string | undefined,
  params: { page?: number; page_size?: number } = {},
) {
  const qs = new URLSearchParams();
  if (params.page) qs.set("page", String(params.page));
  if (params.page_size) qs.set("page_size", String(params.page_size));
  const query = qs.toString();
  return useQuery({
    queryKey: ["groups", id, "members", params],
    queryFn: () =>
      apiFetch<{ data: GroupMember[]; pagination: { total_pages: number; total: number } }>(
        `/groups/${id!}/hosts${query ? `?${query}` : ""}`,
      ),
    enabled: !!id,
  });
}

// ── Mutations ─────────────────────────────────────────────────────────────────

export function useCreateGroup() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: { name: string; description?: string; color?: string }) =>
      apiFetch<HostGroup>("/groups", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["groups"] });
    },
  });
}

export function useUpdateGroup() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({
      id,
      body,
    }: {
      id: string;
      body: { name?: string; description?: string; color?: string };
    }) =>
      apiFetch<HostGroup>(`/groups/${id}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["groups"] });
    },
  });
}

export function useDeleteGroup() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      apiFetch<void>(`/groups/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["groups"] });
    },
  });
}

export function useAddHostsToGroup() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ groupId, hostIds }: { groupId: string; hostIds: string[] }) =>
      apiFetch<{ added: number }>(`/groups/${groupId}/hosts`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ host_ids: hostIds }),
      }),
    onSuccess: (_data, { groupId }) => {
      void queryClient.invalidateQueries({ queryKey: ["groups", groupId] });
      void queryClient.invalidateQueries({ queryKey: ["groups"] });
      void queryClient.invalidateQueries({ queryKey: ["hosts"] });
    },
  });
}

export function useRemoveHostsFromGroup() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ groupId, hostIds }: { groupId: string; hostIds: string[] }) =>
      apiFetch<{ removed: number }>(`/groups/${groupId}/hosts`, {
        method: "DELETE",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ host_ids: hostIds }),
      }),
    onSuccess: (_data, { groupId }) => {
      void queryClient.invalidateQueries({ queryKey: ["groups", groupId] });
      void queryClient.invalidateQueries({ queryKey: ["groups"] });
      void queryClient.invalidateQueries({ queryKey: ["hosts"] });
    },
  });
}
