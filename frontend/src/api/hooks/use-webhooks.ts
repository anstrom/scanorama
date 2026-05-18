import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../client";
import { ApiError } from "../errors";

// ── Types ─────────────────────────────────────────────────────────────────────

export interface WebhookEndpoint {
  id: string;
  url: string;
  secret: string;
  events: string[];
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface WebhookDeliveryLog {
  id: string;
  endpoint_id: string;
  event_type: string;
  status_code: number | null;
  attempt_count: number;
  last_error: string | null;
  delivered_at: string | null;
  created_at: string;
}

export interface CreateWebhookBody {
  url: string;
  secret?: string;
  events: string[];
}

export interface UpdateWebhookBody {
  url?: string;
  secret?: string;
  events?: string[];
  enabled?: boolean;
}

// ── Query keys ────────────────────────────────────────────────────────────────

const WEBHOOKS_KEY = ["webhooks"] as const;
const webhookKey = (id: string) => ["webhooks", id] as const;
const logsKey = (id: string) => ["webhooks", id, "logs"] as const;

// ── Hooks ─────────────────────────────────────────────────────────────────────

export function useWebhooks() {
  return useQuery({
    queryKey: WEBHOOKS_KEY,
    queryFn: async () => {
      const { data, error, response } = await api.GET("/webhooks");
      if (error) throw new ApiError(response.status, error);
      const payload = data as { webhooks?: WebhookEndpoint[] } | undefined;
      return payload?.webhooks ?? [];
    },
    staleTime: 30_000,
  });
}

export function useWebhook(id: string) {
  return useQuery({
    queryKey: webhookKey(id),
    queryFn: async () => {
      const { data, error, response } = await api.GET("/webhooks/{id}", {
        params: { path: { id } },
      });
      if (error) throw new ApiError(response.status, error);
      return data as WebhookEndpoint | undefined;
    },
    enabled: !!id,
    staleTime: 30_000,
  });
}

export function useCreateWebhook() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (body: CreateWebhookBody) => {
      const { data, error, response } = await api.POST("/webhooks", {
        body: body as Parameters<typeof api.POST<"/webhooks">>[1]["body"],
      });
      if (error) throw new ApiError(response.status, error);
      return data as WebhookEndpoint | undefined;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: WEBHOOKS_KEY });
    },
  });
}

export function useUpdateWebhook() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ id, body }: { id: string; body: UpdateWebhookBody }) => {
      const { data, error, response } = await api.PATCH("/webhooks/{id}", {
        params: { path: { id } },
        body: body as Parameters<typeof api.PATCH<"/webhooks/{id}">>[1]["body"],
      });
      if (error) throw new ApiError(response.status, error);
      return data as WebhookEndpoint | undefined;
    },
    onSuccess: (_data, { id }) => {
      queryClient.invalidateQueries({ queryKey: WEBHOOKS_KEY });
      queryClient.invalidateQueries({ queryKey: webhookKey(id) });
    },
  });
}

export function useDeleteWebhook() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (id: string) => {
      const { error, response } = await api.DELETE("/webhooks/{id}", {
        params: { path: { id } },
      });
      if (error) throw new ApiError(response.status, error);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: WEBHOOKS_KEY });
    },
  });
}

export function useTestWebhook() {
  return useMutation({
    mutationFn: async (id: string) => {
      const { error, response } = await api.POST("/webhooks/{id}/test", {
        params: { path: { id } },
      });
      if (error) throw new ApiError(response.status, error);
    },
  });
}

export function useDeliveryLogs(id: string) {
  return useQuery({
    queryKey: logsKey(id),
    queryFn: async () => {
      const { data, error, response } = await api.GET("/webhooks/{id}/logs", {
        params: { path: { id } },
      });
      if (error) throw new ApiError(response.status, error);
      const payload = data as { logs?: WebhookDeliveryLog[] } | undefined;
      return payload?.logs ?? [];
    },
    enabled: !!id,
    staleTime: 10_000,
  });
}
