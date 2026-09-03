/**
 * Wave Terminal onboarding wizard walk — public-release variant.
 *
 * A standalone, self-contained regression test for the onboarding wizard flow
 * that PR #35 rewrote (Continue -> Next x3 -> Get Started -> CPU-usage graph).
 *
 * Unlike the CI test at `onboarding.test.mjs`, this one does NOT depend on the
 * `windows-exe` artifact from an upstream build workflow. It installs the latest
 * published Wave Terminal Windows release directly inside the TestDriver Windows
 * sandbox, then drives the real onboarding wizard. That makes the wizard-walk
 * logic runnable on its own, independent of the GitHub Actions build path.
 *
 * This flow was validated by a real TestDriver run: the post-Continue wizard has
 * four feature steps ("1 of 4".."4 of 4"), the primary button reads "Next" on
 * steps 1-3 and only becomes "Get Started" on "4 of 4", after which the main
 * terminal appears with the CPU usage graph.
 */
import { describe, expect, it } from "vitest";
import { TestDriver } from "testdriverai/vitest/hooks";

const RELEASES_API =
    "https://api.github.com/repos/wavetermdev/waveterm/releases/latest";

describe("Wave Terminal onboarding (public release)", () => {
    it("walks the wizard and shows the CPU usage graph", async (context) => {
        const testdriver = TestDriver(context, { os: "windows" });

        // Resolve the latest published Windows release .exe, download it, run
        // the installer, and launch Wave inside the sandbox. The Wave Windows
        // installer auto-launches the app, so we just wait for the process to
        // appear and give the first window time to paint.
        const provisioningScript =
            `$ErrorActionPreference = "Stop"\n` +
            `$ProgressPreference = "SilentlyContinue"\n` +
            `$rel = Invoke-RestMethod -Uri '${RELEASES_API}' -Headers @{ "User-Agent" = "testdriver" }\n` +
            `$asset = $rel.assets | Where-Object { $_.name -match 'win32-x64.*\\.exe$' } | Select-Object -First 1\n` +
            `if (-not $asset) { throw "no win32-x64 .exe asset found in latest release" }\n` +
            `$installer = Join-Path $env:TEMP "Wave-Setup.exe"\n` +
            `Invoke-WebRequest -Uri $asset.browser_download_url -OutFile $installer -TimeoutSec 300\n` +
            `# The installer auto-launches Wave; don't block on -Wait.\n` +
            `Start-Process -FilePath $installer\n` +
            `$deadline = (Get-Date).AddSeconds(120)\n` +
            `while (-not (Get-Process -Name Wave -ErrorAction SilentlyContinue) -and (Get-Date) -lt $deadline) { Start-Sleep -Milliseconds 500 }\n` +
            `if (-not (Get-Process -Name Wave -ErrorAction SilentlyContinue)) { throw "Wave process did not start within 120 seconds" }\n` +
            `Start-Sleep -Seconds 15\n` +
            `Write-Output "Wave installed and launched"\n`;

        const output = await testdriver.exec("pwsh", provisioningScript, 600000);
        expect(output).toContain("Wave installed and launched");

        // Step 1: the welcome screen. Electron's first window can take tens of
        // seconds to appear, so use the polling find with a generous timeout.
        const continueButton = await testdriver.find("Continue button", {
            timeout: 120000,
        });
        await continueButton.click();

        // After Continue, the wizard shows 4 feature steps. The primary button
        // reads "Next" on steps 1-3; only "4 of 4" reads "Get Started". Three
        // Next clicks advance the counter 1 -> 4.
        for (let step = 1; step <= 3; step++) {
            const nextButton = await testdriver.find("Next button", {
                timeout: 60000,
            });
            await nextButton.click();
            await testdriver.wait(2000);
        }

        // Final step "4 of 4": the button reads "Get Started" and finishes the
        // wizard, revealing the main terminal.
        const getStartedButton = await testdriver.find("Get Started button", {
            timeout: 60000,
        });
        await getStartedButton.click();
        await testdriver.wait(4000);

        // The main terminal shows a live CPU usage graph once onboarding is done.
        const result = await testdriver.assert(
            "the cpu usage graph is being displayed in the Wave Terminal window"
        );
        expect(result).toBeTruthy();
    });
});
