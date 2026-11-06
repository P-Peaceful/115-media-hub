import { fileURLToPath } from "node:url";

import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: { alias: { "@": fileURLToPath(new URL("./src", import.meta.url)) } },
  server: {
    port: 5173,
    strictPort: true,
    proxy: {
      "/health": process.env.MEDIAHUB_BACKEND_URL ?? "http://127.0.0.1:8080",
      "/api": process.env.MEDIAHUB_BACKEND_URL ?? "http://127.0.0.1:8080",
    },
  },
});
