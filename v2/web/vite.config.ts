import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// Dev server proxies the API to the Go backend so the client can use
// same-origin relative URLs (/v1/...) in both dev and production.
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      "/v1": { target: "http://localhost:8080", ws: true, changeOrigin: true },
      "/healthz": "http://localhost:8080",
    },
  },
});
