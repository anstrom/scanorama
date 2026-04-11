import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";

const BASE = import.meta.env.VITE_API_BASE_URL ?? "/api/v1";

async function apiFetch<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${path}`, init);
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw Object.assign(new Error("API error"), { status: res.status, body });
  }
  return res.json() as Promise<T>;
}

// ── Stats summary ─────────────────────────────────────────────────────────────

export interface OSFamilyCount {
  family: string;
  count: number;
}

export interface PortCount {
  port: number;
  count: number;
}

export interface KnowledgeScoreDistribution {
  '0_25': number;
  '25_50': number;
  '50_75': number;
  '75_100': number;
}

export interface StatsSummary {
  hosts_by_status: Record<string, number>;
  hosts_by_os_family: OSFamilyCount[];
  top_ports: PortCount[];
  stale_host_count: number;
  avg_scan_duration_s: number;
  avg_knowledge_score: number;
  knowledge_score_distribution: KnowledgeScoreDistribution;
}

export function useStatsSummary() {
  return useQuery({
    queryKey: ["stats", "summary"],
    queryFn: () => apiFetch<StatsSummary>("/stats/summary"),
    refetchInterval: 60_000,
  });
}

// ── Settings ──────────────────────────────────────────────────────────────────

export interface AppSetting {
  key: string;
  value: string;
  type: string;
  description: string;
  updated_at: string;
}

export function useSettings() {
  return useQuery({
    queryKey: ["admin", "settings"],
    queryFn: () =>
      apiFetch<{ settings: AppSetting[] }>("/admin/settings").then(
        (d) => d.settings,
      ),
    refetchInterval: false,
  });
}

export function useUpdateSetting() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ key, value }: { key: string; value: string }) =>
      apiFetch<{ key: string; updated: boolean }>("/admin/settings", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ key, value }),
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["admin", "settings"] });
    },
  });
}
