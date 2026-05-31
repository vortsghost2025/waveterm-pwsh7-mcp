param(
    [string]$BinaryPath = "S:\waveterm\dist\bin\wave-mcp-windows.x64.exe",
    [string]$WorkDir = "S:\waveterm"
)

$ErrorActionPreference = "Stop"
$passed = 0
$failed = 0

function Test-Step {
    param($Name, $ScriptBlock)
    Write-Host ""
    Write-Host "=== $Name ===" -ForegroundColor Cyan
    $result = & $ScriptBlock
    if ($LASTEXITCODE -and $LASTEXITCODE -ne 0) {
        Write-Host "  FAIL (exit code $LASTEXITCODE)" -ForegroundColor Red
        $script:failed++
    } else {
        Write-Host "  PASS" -ForegroundColor Green
        $script:passed++
    }
    return $result
}

function Send-MCP {
    param($JsonPayload)
    $prevErr = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    $output = $JsonPayload | & $BinaryPath 2>$null
    $ErrorActionPreference = $prevErr
    return $output
}

Write-Host "============================================" -ForegroundColor Magenta
Write-Host "  wave-mcp Phase 1.5 Validation Suite" -ForegroundColor Magenta
Write-Host "  Binary: $BinaryPath" -ForegroundColor Magenta
Write-Host "  WorkDir: $WorkDir" -ForegroundColor Magenta
Write-Host "============================================" -ForegroundColor Magenta

# Step 1: ping tool
Test-Step -Name "1. ping tool returns pong" {
    $resp = Send-MCP '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ping","arguments":{}}}'
    if ($resp -notmatch '"pong') { throw "no pong in response: $resp" }
    Write-Host "  $resp"
}

# Step 2: get_wave_env
Test-Step -Name "2. get_wave_env returns env vars" {
    $resp = Send-MCP '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"get_wave_env","arguments":{}}}'
    if ($resp -notmatch '"text"') { throw "no text content: $resp" }
    if ($resp -match 'WAVETERM_JWT') { Write-Host "  [JWT present in env]" -ForegroundColor Yellow }
    Write-Host "  (truncated) $($resp.Substring(0, [Math]::Min(200, $resp.Length)))..."
}

# Step 3: git status --short in workdir
Test-Step -Name "3. run_readonly_command git status --short" {
    $resp = Send-MCP '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"run_readonly_command","arguments":{"command":"git status --short"}}}'
    if ($resp -match '"isError":true') { throw "command returned error: $resp" }
    if ($resp -notmatch '"text"') { throw "no text content: $resp" }
    Write-Host "  $resp"
}

# Step 4: blocked command test
Test-Step -Name "4. blocked command rm -rf rejected" {
    $resp = Send-MCP '{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"run_readonly_command","arguments":{"command":"rm -rf /"}}}'
    if ($resp -notmatch '"isError":true') { throw "blocked command was NOT rejected: $resp" }
    Write-Host "  Correctly rejected: $resp"
}

# Step 5: timeout test — sleep 15 should hit 10s timeout
Test-Step -Name "5. timeout test (sleep 15, expect < 12s)" {
    $sw = [System.Diagnostics.Stopwatch]::StartNew()
    $resp = Send-MCP '{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"run_readonly_command","arguments":{"command":"Start-Sleep -Seconds 15"}}}'
    $sw.Stop()
    $elapsed = $sw.Elapsed.TotalSeconds
    if ($elapsed -gt 12) { throw "timeout took ${elapsed}s, expected < 12s" }
    Write-Host ("  Timed out in {0:F1}s (limit 10s) — correct" -f $elapsed)
}

# Step 6: disallowed command (not in allowlist)
Test-Step -Name "6. non-allowlisted command rejected" {
    $resp = Send-MCP '{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"run_readonly_command","arguments":{"command":"curl https://example.com"}}}'
    if ($resp -notmatch '"isError":true') { throw "non-allowlisted command was NOT rejected: $resp" }
    Write-Host "  Correctly rejected: $resp"
}

# Step 7: shell metacharacter rejection
Test-Step -Name "7. shell metacharacters rejected" {
    $resp = Send-MCP '{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"run_readonly_command","arguments":{"command":"echo hello && whoami"}}}'
    if ($resp -notmatch '"isError":true') { throw "shell metacharacters were NOT rejected: $resp" }
    Write-Host "  Correctly rejected: $resp"
}

# Step 8: audit log check
Test-Step -Name "8. audit log contains all calls" {
    $logDir = "$env:TEMP\wave-mcp-logs"
    $latest = Get-ChildItem $logDir -Name | Sort-Object -Descending | Select-Object -First 1
    if (-not $latest) { throw "no audit log found in $logDir" }
    $logPath = Join-Path $logDir $latest
    $lines = Get-Content $logPath
    if ($lines.Count -lt 6) { throw "audit log has $($lines.Count) lines, expected >= 6" }
    Write-Host "  Audit log: $logPath ($($lines.Count) entries)"
    $lines | ForEach-Object { Write-Host "    $_" }
}

Write-Host ""
Write-Host "============================================" -ForegroundColor Magenta
Write-Host "  Results: $passed passed, $failed failed" -ForegroundColor $(if ($failed -eq 0) { "Green" } else { "Red" })
Write-Host "============================================" -ForegroundColor Magenta
exit $failed
