/**
 * Wave Terminal onboarding test (TestDriver SDK + Vitest).
 *
 * Modern replacement for the legacy `testdriver/onboarding.yml` runbook, which
 * is preserved unchanged for reference. Executed by the "TestDriver.ai Run"
 * workflow (`.github/workflows/testdriver.yml`) from the repo's `testdriver/`
 * working directory with `testdriverai` and `vitest` installed alongside it
 * (dependency setup lives in the workflow).
 *
 * Artifact transport (runner -> TestDriver sandbox):
 * The `windows-exe` artifact built by the triggering "TestDriver.ai Build"
 * workflow_run is Downloaded by the TestDriver sandbox over GitHub's own
 * short-lived, signature-authenticated artifact URL (the 302 redirect target
 * of the artifact download endpoint). The URL is resolved HERE, in this
 * runner-side Vitest process, using GITHUB_TOKEN - the token never leaves the
 * runner. Only the signed URL (equivalent to the artifact bytes themselves,
 * which are not secret) is passed to the sandbox. No third-party file host is
 * involved.
 */
import { describe, expect, it } from "vitest";
import { TestDriver } from "testdriverai/vitest/hooks";

const GITHUB_API = "https://api.github.com";
const ARTIFACT_NAME = "windows-exe";

/**
 * Resolve a short-lived signed download URL for the triggering run's
 * windows-exe artifact. Runs on the GitHub Actions runner.
 */
async function resolveArtifactUrl() {
    const runId = process.env.WAVETERM_BUILD_RUN_ID;
    const token = process.env.GH_TOKEN;
    const repo = process.env.GITHUB_REPOSITORY;
    if (!runId || !token || !repo) {
        throw new Error(
            "WAVETERM_BUILD_RUN_ID, GH_TOKEN, and GITHUB_REPOSITORY must be " +
                "set (exported by the TestDriver.ai Run workflow); they are " +
                "used on the runner to resolve the artifact's signed URL."
        );
    }

    const authHeaders = {
        Authorization: `Bearer ${token}`,
        Accept: "application/vnd.github+json",
        "X-GitHub-Api-Version": "2022-11-28",
    };
    // Fail fast if the GitHub API hangs, instead of stalling the test until
    // the vitest timeout.
    const requestTimeout = AbortSignal.timeout(30000);

    const res = await fetch(
        `${GITHUB_API}/repos/${repo}/actions/runs/${runId}/artifacts`,
        { headers: authHeaders, signal: requestTimeout }
    );
    if (!res.ok) {
        throw new Error(
            `Failed to list artifacts for run ${runId}: HTTP ${res.status}`
        );
    }
    const body = await res.json();
    const artifact = (body.artifacts ?? []).find(
        (a) => a.name === ARTIFACT_NAME
    );
    if (!artifact) {
        throw new Error(
            `Artifact '${ARTIFACT_NAME}' not found on workflow run ${runId}.`
        );
    }

    // Follow the artifact download endpoint's redirect manually so we capture
    // the signed URL instead of downloading the payload on the runner.
    const dl = await fetch(artifact.archive_download_url, {
        headers: authHeaders,
        redirect: "manual",
    });
    const signedUrl = dl.headers.get("location");
    if (dl.status !== 302 || !signedUrl) {
        throw new Error(
            `Expected a 302 with a signed Location header from the artifact ` +
                `download endpoint, got HTTP ${dl.status}.`
        );
    }
    return signedUrl;
}

describe("Wave Terminal onboarding", () => {
    it("completes onboarding and shows the CPU usage graph", async (context) => {
        const artifactUrl = await resolveArtifactUrl();
        const testdriver = TestDriver(context, { os: "windows" });

        // Download the build artifact, install, and launch Wave Terminal
        // inside the TestDriver Windows sandbox. Mirrors the legacy
        // testdriver.ai Run "prerun" script. The URL is GitHub's signed
        // artifact URL, resolved runner-side seconds ago.
        const provisioningScript =
            `$ErrorActionPreference = "Stop"\n` +
            `$ProgressPreference = "SilentlyContinue"\n` +
            `$url = '${artifactUrl}'\n` +
            `$zipPath = Join-Path $env:TEMP "wave-installer.zip"\n` +
            `Invoke-WebRequest -Uri $url -OutFile $zipPath -TimeoutSec 300\n` +
            `$dest = Join-Path $env:TEMP "wave-installer"\n` +
            `Expand-Archive -Path $zipPath -DestinationPath $dest -Force\n` +
            `$installer = Get-ChildItem -Path $dest -Filter *.exe -Recurse | Select-Object -First 1\n` +
            `if (-not $installer) { throw "no installer .exe found inside artifact" }\n` +
            `Start-Process -FilePath $installer.FullName -Wait\n` +
            `$wave = Join-Path $env:USERPROFILE "AppData\\Local\\Programs\\waveterm\\Wave.exe"\n` +
            `Start-Process -FilePath $wave\n` +
            `# Wait for the Electron process to appear (up to 60s), then give the\n` +
            `# first window time to render before the script returns. Without this,\n` +
            `# Start-Process returns immediately and the onboarding flow is not yet\n` +
            `# on screen when the first element lookup runs.\n` +
            `$deadline = (Get-Date).AddSeconds(60)\n` +
            `while (-not (Get-Process -Name Wave -ErrorAction SilentlyContinue) -and (Get-Date) -lt $deadline) { Start-Sleep -Milliseconds 500 }\n` +
            `if (-not (Get-Process -Name Wave -ErrorAction SilentlyContinue)) { throw "Wave process did not start within 60 seconds" }\n` +
            `Start-Sleep -Seconds 15\n` +
            `Write-Output "Wave installed and launched"\n`;

        const output = await testdriver.exec("pwsh", provisioningScript, 600000);
        expect(output).toContain("Wave installed and launched");

        // On any failure below, dump the app's own log tail and process state
        // from the sandbox into the test output. The onboarding window
        // sometimes never paints on a fresh sandbox; waveapp.log shows
        // whether Electron was still parked waiting for the wavesrv
        // ESTART signal (no window path) or errored elsewhere, which
        // turns every future flake into a self-diagnosing run.
        const dumpEvidence = async (label) => {
            try {
                const evidence = await testdriver.exec(
                    "pwsh",
                    `$log = Join-Path $env:LOCALAPPDATA "waveterm\\waveapp.log"\n` +
                    `Write-Output "=== evidence: ${label} ==="\n` +
                    `Write-Output "--- processes ---"\n` +
                    `Get-Process -Name Wave,wavesrv -ErrorAction SilentlyContinue | Select-Object Name,Id | Format-Table | Out-String\n` +
                    `Write-Output "--- waveapp.log tail ---"\n` +
                    `if (Test-Path $log) { Get-Content $log -Tail 40 } else { Write-Output "(waveapp.log not found)" }\n`,
                    60000
                );
                console.log(evidence);
            } catch (e) {
                console.log(`evidence dump failed (${label}):`, e?.message ?? e);
            }
        };

        try {
            // Port of the legacy runbook (testdriver/onboarding.yml), walking
            // the real wizard flow. Electron's first window can take tens of
            // seconds to appear after Start-Process, so use the SDK's polling
            // find (retries every 5s) with a generous timeout.
            const continueButton = await testdriver.find(
                "Continue button of the Wave Terminal onboarding flow",
                { timeout: 120000 }
            );
            await continueButton.click();

            // After Continue, the wizard shows 4 feature steps. The button on
            // steps 1-3 is labeled "Next" (only the final step's button reads
            // "Get Started"), so click through the wizard with that
            // description; the SDK's AI locator matches the primary action
            // button of the current step.
            for (let step = 1; step <= 3; step++) {
                const nextButton = await testdriver.find(
                    "Next button of the Wave Terminal onboarding wizard",
                    { timeout: 60000 }
                );
                await nextButton.click();
            }

            // Final step: the button reads "Get Started" and finishes the
            // wizard, revealing the main terminal with the CPU usage graph.
            const getStartedButton = await testdriver.find(
                "Get Started button of the Wave Terminal onboarding flow",
                { timeout: 60000 }
            );
            await getStartedButton.click();

            // assert the CPU usage graph is displayed
            const result = await testdriver.assert(
                "the cpu usage graph is being displayed"
            );
            expect(result).toBeTruthy();
        } catch (e) {
            await dumpEvidence("onboarding failure");
            throw e;
        }
    });
});
