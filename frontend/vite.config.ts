/// <reference types="vitest/config" />
import path from "node:path";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

// 127.0.0.1 вместо localhost: VPN часто ломает резолв localhost / ::1
const LOCAL = "127.0.0.1";

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      "@": path.resolve(import.meta.dirname, "src"),
    },
  },
  server: {
    host: LOCAL,
    port: 5173,
    strictPort: true,
    hmr: { host: LOCAL },
    proxy: {
      "/api": { target: `http://${LOCAL}:8080`, changeOrigin: true },
    },
  },
  preview: {
    host: LOCAL,
    port: 3000,
    strictPort: true,
  },
  test: {
    environment: "jsdom",
    setupFiles: ["./src/test/setup.ts"],
    // Интеграционные сценарии печатают текст и проходят несколько экранов; под подсчётом покрытия они медленнее.
    testTimeout: 15000,
    include: ["src/**/*.test.{ts,tsx}"],
    coverage: {
      provider: "v8",
      reporter: ["text", "html", "json-summary", "lcov"],
      reportsDirectory: "coverage",
      // Весь код приложения, который загружает index.html (src/app/main.tsx и всё, что он импортирует).
      // Не считаются только тесты, тестовые помощники, точка входа и barrel-файлы с одними реэкспортами.
      include: ["src/**/*.{ts,tsx}"],
      exclude: ["src/**/*.test.{ts,tsx}", "src/test/**", "src/app/main.tsx", "src/**/index.ts"],
      thresholds: { lines: 80, functions: 80, statements: 80, branches: 80 },
    },
  },
});
