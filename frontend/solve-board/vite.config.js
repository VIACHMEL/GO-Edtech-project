import { fileURLToPath } from "node:url";
import { defineConfig } from "vite";

export default defineConfig({
    base: "./",
    build: {
        outDir: "../static/solve-board/dist",
        emptyOutDir: true,
        rollupOptions: {
            input: fileURLToPath(new URL("./src/solve-board.js", import.meta.url)),
            output: {
                entryFileNames: "solve-board.js",
            },
        },
    },
});
