import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { RouterProvider } from "@tanstack/react-router";
import { router } from "./router";
import { WsProvider } from "./lib/use-ws";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      refetchOnWindowFocus: false,
      staleTime: 10_000,
    },
  },
});

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <WsProvider apiKey={import.meta.env.VITE_API_KEY ?? ""}>
        <RouterProvider router={router} />
      </WsProvider>
    </QueryClientProvider>
  );
}
