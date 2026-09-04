import path from "path"
import { fileURLToPath } from "url"
import react from "@vitejs/plugin-react"
import { defineConfig } from "vite"

// Builds the headless report-preview page runtime (src/report-preview/main.ts)
// as ONE self-contained IIFE the Go server serves from disk at
// /report-preview/report-preview.js (report_preview_routes.go), the same way
// it serves the rest of the SPA -- not go:embed'd, since agent_go/cmd/server/
// static/ is a deploy-populated, uncommitted directory. It shares the report
// host runtime and markdown renderer with the in-app Report tab, so the
// preview renders a report exactly the way the app does.
//
// Output goes here (so a local `go run ./cmd/server` finds it without a full
// frontend build) AND, via the `build:report-preview` npm script, gets
// copied into frontend/dist/ alongside the main app bundle -- every existing
// deploy script already copies frontend/dist/ wholesale into the deployed
// static root, so this rides along with the ordinary frontend build with no
// deploy-script change.
const projectRoot = path.dirname(fileURLToPath(import.meta.url))

export default defineConfig({
  plugins: [react()],
  // The app's public/ assets belong to the SPA build, not to this bundle.
  publicDir: false,
  resolve: {
    alias: {
      "@": path.resolve(projectRoot, "./src"),
    },
  },
  define: {
    'process.env.NODE_ENV': '"production"',
    'import.meta.env.DEV': 'false',
  },
  build: {
    outDir: path.resolve(projectRoot, "../agent_go/cmd/server/static"),
    emptyOutDir: false,
    minify: true,
    sourcemap: false,
    lib: {
      entry: path.resolve(projectRoot, "src/report-preview/main.ts"),
      name: "ReportPreview",
      formats: ["iife"],
      fileName: () => "report-preview.js",
    },
    rollupOptions: {
      output: { inlineDynamicImports: true },
    },
  },
})
