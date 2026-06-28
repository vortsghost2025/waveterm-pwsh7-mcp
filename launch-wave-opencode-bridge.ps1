<#
.SYNOPSIS
    Bridge launcher — runs opencode INSIDE a Wave Alt terminal tab.
    opencode inherits WAVETERM_JWT from the Wave shell, enabling wave-mcp.
.DESCRIPTION
    Run this script from inside a Wave Alt terminal tab.
    It launches opencode with the same config, but wave-mcp will connect
    successfully because WAVETERM_JWT is available.
#>

param(
    [string]$WorkingDir = "S:\waveterm"
)

if (-not $env:WAVETERM_JWT) {
    Write-Host "`n[bridge] WARNING: WAVETERM_JWT not found." -ForegroundColor Yellow
    Write-Host "[bridge] This script must be run from INSIDE a Wave terminal tab." -ForegroundColor Yellow
    Write-Host "[bridge] wave-mcp will not work without the JWT.`n" -ForegroundColor Yellow
}

Set-Location $WorkingDir
opencode
