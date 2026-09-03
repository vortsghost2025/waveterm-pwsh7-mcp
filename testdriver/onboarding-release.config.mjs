import { defineConfig } from "vitest/config";
import TestDriver from "testdriverai/vitest";
import { config } from "dotenv";

// Load TD_API_KEY / TD_CHANNEL from the repo-root .env.
config({ path: new URL("../.env", import.meta.url).pathname });

// Standalone config for the public-release onboarding walk. Kept separate from
// the repo's app-level vitest.config.ts (renderer/electron unit tests) and from
// the CI-artifact onboarding.test.mjs so this can run on its own without an
// upstream build workflow.
export default defineConfig({
  test: {
    include: ["onboarding-release.test.mjs"],
    testTimeout: 900000,
    hookTimeout: 900000,
    reporters: ["default", TestDriver()],
    setupFiles: ["testdriverai/vitest/setup"],
  },
});
