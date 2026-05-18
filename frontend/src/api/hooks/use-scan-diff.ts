import { useQuery } from "@tanstack/react-query";
import { api } from "../client";
import { ApiError } from "../errors";
import type { components } from "../types";

export type ScanDiffEntry = components["schemas"]["docs.ScanDiffEntryResponse"];
export type ScanDiff = components["schemas"]["docs.ScanDiffResponse"];

/**
 * useScanDiff fetches a diff between two scans of the same host.
 *
 * @param scanAId - Baseline scan ID (older)
 * @param scanBId - Current scan ID (newer)
 *
 * Enabled only when both IDs are provided. Diffs are stable once computed
 * (scan results don't change), so staleTime is set to 60 s.
 */
export function useScanDiff(
  scanAId: string | undefined,
  scanBId: string | undefined,
) {
  return useQuery({
    queryKey: ["scans", "diff", scanAId, scanBId],
    queryFn: async () => {
      const { data, error, response } = await api.GET("/scans/diff", {
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        params: { query: { a: scanAId!, b: scanBId! } as any },
      });
      if (error) throw new ApiError(response.status, error);
      return data as ScanDiff;
    },
    enabled: !!scanAId && !!scanBId,
    staleTime: 60_000,
  });
}
