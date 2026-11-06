import { defineConfig, type Plugin } from "vite";
import react from "@vitejs/plugin-react";

// In development (npm run dev) Vite serves index.html instead of the Go server,
// so it asks the Go server for the language and writes it into <html lang>,
// the same way backend/server.go does in production.
const languageFromBackend: Plugin = {
  name: "bancada-language",
  apply: "serve",
  async transformIndexHtml(html) {
    try {
      const resp = await fetch("http://127.0.0.1:8080/api/settings");
      const settings = await resp.json();
      return html.replace('<html lang="pt-BR">', `<html lang="${settings.config.language}">`);
    } catch {
      return html; // Go server not running yet: keep Portuguese
    }
  },
};

// The build goes straight into the Go backend, which embeds it into the binary.
// During development API calls are forwarded to the Go server.
export default defineConfig({
  plugins: [react(), languageFromBackend],
  build: { outDir: "../backend/web/dist", emptyOutDir: true },
  server: { proxy: { "/api": "http://127.0.0.1:8080" } },
});
