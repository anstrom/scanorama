import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../client";
import type { components } from "../types";

// ── Types ─────────────────────────────────────────────────────────────────────

export type AlertRule = components["schemas"]["docs.AlertRuleResponse"];

// The swagger spec reuses AlertRuleResponse for request bodies; these local
// aliases document the intended subset until the spec is updated.
export type CreateAlertRuleBody = Pick<
  AlertRule,
  "host_id" | "group_id" | "tag" | "trigger" | "channel_url"
>;
export type UpdateAlertRuleBody = Pick<
  AlertRule,
  "trigger" | "channel_url" | "enabled"
>;

// ── Query keys ────────────────────────────────────────────────────────────────

const ALERTS_KEY = ["alerts"] as const;
const hostAlertsKey = (hostID: string) => ["alerts", "host", hostID] as const;

// ── Hooks ─────────────────────────────────────────────────────────────────────

export function useAlertRules() {
  return useQuery({
    queryKey: ALERTS_KEY,
    queryFn: async () => {
      const { data, error } = await api.GET("/alerts");
      if (error) throw error;
      return data?.alert_rules ?? [];
    },
    staleTime: 30_000,
  });
}

export function useHostAlertRules(hostID: string) {
  return useQuery({
    queryKey: hostAlertsKey(hostID),
    queryFn: async () => {
      const { data, error } = await api.GET("/hosts/{id}/alerts", {
        params: { path: { id: hostID } },
      });
      if (error) throw error;
      return data?.alert_rules ?? [];
    },
    enabled: !!hostID,
    staleTime: 30_000,
  });
}

export function useCreateAlertRule() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (body: CreateAlertRuleBody) => {
      const { data, error } = await api.POST("/alerts", { body });
      if (error) throw error;
      return data;
    },
    onSuccess: (_data, vars) => {
      queryClient.invalidateQueries({ queryKey: ALERTS_KEY });
      if (vars.host_id) {
        queryClient.invalidateQueries({
          queryKey: hostAlertsKey(vars.host_id),
        });
      }
    },
  });
}

export function useUpdateAlertRule() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({
      id,
      body,
    }: {
      id: string;
      body: UpdateAlertRuleBody;
    }) => {
      const { data, error } = await api.PATCH("/alerts/{id}", {
        params: { path: { id } },
        body,
      });
      if (error) throw error;
      return data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ALERTS_KEY });
    },
  });
}

export function useDeleteAlertRule() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ id }: { id: string; hostID?: string }) => {
      const { error } = await api.DELETE("/alerts/{id}", {
        params: { path: { id } },
      });
      if (error) throw error;
    },
    onSuccess: (_data, { hostID }) => {
      queryClient.invalidateQueries({ queryKey: ALERTS_KEY });
      if (hostID) {
        queryClient.invalidateQueries({ queryKey: hostAlertsKey(hostID) });
      }
    },
  });
}
