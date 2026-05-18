import { useQuery } from "@tanstack/react-query";
import { api } from "../client";
import { ApiError } from "../errors";

export interface SearchResultItem {
  id: string;
  label: string;
  url: string;
  type: string;
}

export interface SearchResults {
  results: {
    hosts?: SearchResultItem[];
    networks?: SearchResultItem[];
    scans?: SearchResultItem[];
    profiles?: SearchResultItem[];
  };
  total: number;
}

const SEARCH_LIMIT = 10;

/**
 * Fires GET /api/v1/search?q=<query>&limit=10.
 * Enabled only when query has at least 2 characters.
 * staleTime: 0 — search results should always be fresh.
 */
export function useSearch(query: string) {
  const enabled = query.length >= 2;

  return useQuery<SearchResults>({
    queryKey: ["search", query],
    queryFn: async () => {
      const { data, error, response } = await api.GET("/search", {
        params: { query: { q: query, limit: SEARCH_LIMIT } },
      });
      if (error) throw new ApiError(response.status, error);
      return data as SearchResults;
    },
    enabled,
    staleTime: 0,
  });
}
