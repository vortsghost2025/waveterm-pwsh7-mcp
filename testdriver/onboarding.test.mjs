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

    const res = await fetch(
        `${GITHUB_API}/repos/${repo}/actions/runs/${runId}/artifacts`,
        { headers: authHeaders }
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
            `Write-Output "Wave installed and launched"\n`;

        const output = await testdriver.exec("pwsh", provisioningScript, 600000);
        expect(output).toContain("Wave installed and launched");

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
