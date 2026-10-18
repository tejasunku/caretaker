import { defineConfig } from "vite";
import { nitro } from "nitro/vite";
import { solidStart } from "@solidjs/start/config";
import UnoCSS from "unocss/vite";
import { caretakerPatchApi } from "./patch-api";

export default defineConfig({
  plugins: [
    UnoCSS(),
    solidStart({
      ssr: false,
    }),
    nitro(),
    caretakerPatchApi(),
  ],
});
