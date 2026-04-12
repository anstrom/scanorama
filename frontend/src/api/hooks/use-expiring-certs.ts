import { useQuery } from "@tanstack/react-query";

const BASE = import.meta.env.VITE_API_BASE_URL ?? "/api/v1";

async function apiFetch<T>(path: string): Promise<T> {
  const res = await fetch(`${BASE}${path}`);
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw Object.assign(new Error("API error"), { status: res.status, body });
  }
  return res.json() as Promise<T>;
}

export interface ExpiringCertificate {
  host_id: string;
  host_ip: string;
  hostname: string;
  port: number;
  protocol: string;
  subject_cn: string;
  not_after: string;
  days_left: number;
}

export interface ExpiringCertificatesResponse {
  certificates: ExpiringCertificate[];
  days: number;
}

export function useExpiringCerts(days = 30) {
  return useQuery({
    queryKey: ["certificates", "expiring", days],
    queryFn: () =>
      apiFetch<ExpiringCertificatesResponse>(
        `/certificates/expiring?days=${days}`,
      ),
    refetchInterval: 5 * 60_000, // refresh every 5 minutes
  });
}
