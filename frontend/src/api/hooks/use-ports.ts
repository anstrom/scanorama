import { useQuery } from "@tanstack/react-query";
import { ApiError } from "../errors";

export interface PortDefinition {
  port: number;
  protocol: string;
  service: string;
  description?: string;
  category?: string;
  os_families?: string[];
  is_standard: boolean;
}

export interface PortListResponse {
  ports: PortDefinition[] | null;
  total: number;
  page: number;
  page_size: number;
  total_pages: number;
}

interface PortListParams {
  search?: string;
  category?: string;
  protocol?: string;
  sort_by?: string;
  sort_order?: "asc" | "desc";
  page?: number;
  page_size?: number;
}

const baseURL = () => import.meta.env.VITE_API_BASE_URL ?? "/api/v1";

export function usePorts(params: PortListParams = {}) {
  return useQuery({
    queryKey: ["ports", params],
    queryFn: async () => {
      const url = new URL(`${baseURL()}/ports`, window.location.origin);
      Object.entries(params).forEach(([k, v]) => {
        if (v !== undefined && v !== "") url.searchParams.set(k, String(v));
      });

      const res = await fetch(url.toString());
      if (!res.ok) {
        const body = await res.json().catch(() => ({}));
        throw new ApiError(res.status, body);
      }
      return res.json() as Promise<PortListResponse>;
    },
  });
}

export function usePortCategories() {
  return useQuery({
    queryKey: ["ports", "categories"],
    queryFn: async () => {
      const res = await fetch(`${baseURL()}/ports/categories`);
      if (!res.ok) {
        const body = await res.json().catch(() => ({}));
        throw new ApiError(res.status, body);
      }
      const data = (await res.json()) as { categories: string[] | null };
      return data.categories ?? [];
    },
  });
}
