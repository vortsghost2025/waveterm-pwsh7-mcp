# Send-WaveNudge.ps1
# Sends a WAVE_NUDGE v1 payload to Wave AI via wsh ai -s -m and logs the attempt.

param(
    [Parameter(Mandatory=$false)]
    [string]$Message = "Nudge"
)

$wshPath = "S:\waveterm\dist\bin\wsh.exe"
$logFile = "S:\waveterm\agent-coordination\nudge-log.txt"
$timestamp = Get-Date -Format o

# Build the WAVE_NUDGE v1 payload
$payload = @{
    timestamp = $timestamp
    sender    = "opencode"
    message   = $Message
} | ConvertTo-Json -Depth 3

$payloadText = "WAVE_NUDGE v1`n$payload"

# Execute wsh ai -s -m with the payload
& $wshPath ai -s -m $payloadText
$exitCode = $LASTEXITCODE

# Log the attempt
$logEntry = "$timestamp | Sent: $Message | ExitCode: $exitCode"
Add-Content -Path $logFile -Value $logEntry

if ($exitCode -eq 0) {
    Write-Host "Nudge sent successfully."
} else {
    Write-Warning "Failed to send nudge. Exit code: $exitCode"
}