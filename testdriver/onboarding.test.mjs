/**
 * Wave Terminal onboarding test (TestDriver SDK + Vitest).
 *
 * Modern replacement for the legacy `testdriver/onboarding.yml` runbook, which
 * is preserved unchanged for reference. This file is executed by the
 * "TestDriver.ai Run" workflow (`.github/workflows/testdriver.yml`) from the
 * repo's `testdriver/` working directory with `testdriverai` and `vitest`
 * installed alongside it (dependency setup lives in the workflow).
 *
 * The workflow provisions this run from the `windows-exe` artifact of the
 * triggering `workflow_run` and exposes it as `WAVETERM_INSTALLER_URL`.
 */
import { describe, expect, it } from "vitest";
import { TestDriver } from "testdriverai/vitest/hooks";

const installerUrl = process.env.WAVETERM_INSTALLER_URL;

if (!installerUrl) {
    throw new Error(
        "WAVETERM_INSTALLER_URL is not set. This test installs and onboards " +
            "the Wave Terminal Windows build produced by the triggering " +
            "'TestDriver.ai Build' workflow run; the installer URL is exported " +
            "by the 'TestDriver.ai Run' workflow."
    );
}

describe("Wave Terminal onboarding", () => {
    it("completes onboarding and shows the CPU usage graph", async (context) => {
        const testdriver = TestDriver(context, { os: "windows" });

        // Download, install, and launch the Wave Terminal build under test.
        await testdriver.provision.installer({
            url: installerUrl,
            filename: "waveterm-setup.exe",
        });

        // Port of the legacy runbook (testdriver/onboarding.yml):
        // 1. click "Continue"
        const continueButton = await testdriver.find(
            "Continue button of the Wave Terminal onboarding flow"
        );
        await continueButton.click();

        // 2. click "Get Started"
        const getStartedButton = await testdriver.find(
            "Get Started button of the Wave Terminal onboarding flow"
        );
        await getStartedButton.click();

        // 3. assert the CPU usage graph is displayed
        const result = await testdriver.assert(
            "the cpu usage graph is being displayed"
        );
        expect(result).toBeTruthy();
    });
});
