import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import { fileURLToPath } from "node:url";

// 后端地址：默认本机 3002
const API_TARGET = process.env.API_TARGET ?? "http://127.0.0.1:3002";

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      "@": fileURLToPath(new URL("./src", import.meta.url)),
    },
  },
  server: {
    host: "0.0.0.0",
    port: 5173,
    proxy: {
      // 前端一律请求同源 /api 与 /v1，由 Vite 转发到 Express，避免跨域
      "/api": { target: API_TARGET, changeOrigin: true },
      "/v1": { target: API_TARGET, changeOrigin: true },
    },
  },
  preview: {
    host: "0.0.0.0",
    port: 5180,
    proxy: {
      "/api": { target: API_TARGET, changeOrigin: true },
      "/v1": { target: API_TARGET, changeOrigin: true },
    },
  },
});
