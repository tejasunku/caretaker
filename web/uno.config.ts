import { defineConfig, presetWind } from "unocss";

export default defineConfig({
  presets: [presetWind()],
  theme: {
    colors: {
      bg: "#ffffff",
      "sidebar-bg": "#f8f9fa",
      "content-bg": "#ffffff",
      border: "#e2e5ea",
      text: "#1a1d2e",
      "text-dim": "#6b7280",
      accent: "#4f5bd5",
      green: "#16a34a",
      yellow: "#ca8a04",
      orange: "#ea580c",
      red: "#dc2626",
      blue: "#2563eb",
      gray: "#9ca3af",
      "card-bg": "#ffffff",
      "card-border": "#e2e5ea",
    },
  },
});
