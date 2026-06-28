# Spawn a new agent terminal in Wave
# Usage: .\scripts\spawn-agent.ps1 [-model waveai@quick] [-cwd S:\waveterm]
param(
    [string]$model = "waveai@quick",
    [string]$cwd = "S:\waveterm"
)
$cmd = "Set-Location '$cwd'; opencode --model $model"
$result = wsh run -c $cmd -m 2>&1
if ($LASTEXITCODE -ne 0) {
    Write-Error "Failed to spawn agent: $result"
    exit 1
}
Write-Output "Agent spawned: $result"
