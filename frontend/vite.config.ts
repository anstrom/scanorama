import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  server: {
    host: "0.0.0.0",
    port: 3000,
    strictPort: true,
    watch: {
      usePolling: false,
      useFsEvents: true,
    },
    proxy: {
      "/api/v1": {
        target: "http://localhost:8080",
        changeOrigin: true,
        ws: true, // Enable WebSocket proxying
        secure: false,
        timeout: 10000,
        configure: (proxy, _options) => {
          proxy.on("error", (err, _req, _res) => {
            console.error("Proxy error:", err.message);
          });
          proxy.on("proxyReqWs", (_proxyReq, _req, socket) => {
            console.log("WebSocket proxy request");
            socket.on("error", (err) => {
              console.error("WebSocket proxy socket error:", err.message);
            });
          });
        },
      },
    },
  },
  optimizeDeps: {
    include: ["react", "react-dom"],
  },
  build: {
    target: "esnext",
    minify: "esbuild",
    rollupOptions: {
      output: {
        manualChunks: {
          vendor: ["react", "react-dom"],
          router: ["react-router-dom"],
          icons: ["lucide-react"],
        },
      },
    },
  },
  resolve: {
    alias: {
      "@": "/src",
    },
  },
  define: {
    __DEV__: JSON.stringify(process.env["NODE_ENV"] === "development"),
  },
});
