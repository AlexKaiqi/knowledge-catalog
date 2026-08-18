import { defineConfig } from "vitest/config";

export default defineConfig({
  test: {
    // Protocol conformance lives at repo root. The scene worktree is a
    // separate tree and must not be collected (stale copies, extra collectors).
    exclude: ["**/node_modules/**", "**/.scenes/**", "**/dist/**"],
  },
});
