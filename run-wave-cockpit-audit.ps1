$out = "S:\waveterm\WAVE-COCKPIT-AUDIT.txt"
"=== WAVE COCKPIT AUDIT ===" | Set-Content $out
"Time: $(Get-Date)" | Add-Content $out

"`n=== package scripts ===" | Add-Content $out
Get-Content .\package.json -Raw | Add-Content $out

"`n=== launch scripts ===" | Add-Content $out
Get-Content .\launch-wave-alt.ps1 -Raw | Add-Content $out
"`n--- launch-wave-bridge.ps1 ---" | Add-Content $out
Get-Content .\launch-wave-bridge.ps1 -Raw | Add-Content $out

"`n=== executables ===" | Add-Content $out
Get-Item .\wave.exe,.\wsh.exe,.\server.exe -ErrorAction SilentlyContinue |
  Select-Object FullName, LastWriteTime, Length |
  Format-Table -AutoSize |
  Out-String -Width 300 |
  Add-Content $out

"`n=== source hits: terminal/profile/shell/split ===" | Add-Content $out
$patterns = "localshellpath","terminal profile","profiles","new terminal","split","wsh launch","shellpath","Command Prompt","PowerShell","Git Bash","wsl"
$files = Get-ChildItem .\frontend,.\emain,.\pkg,.\cmd -Recurse -File -ErrorAction SilentlyContinue |
  Where-Object { $_.Extension -match '\.(ts|tsx|js|jsx|go|json)$' -and $_.FullName -notmatch '\\node_modules\\|\\dist\\|\\build\\' }

foreach ($p in $patterns) {
  "`n--- PATTERN: $p ---" | Add-Content $out
  Select-String -Path $files.FullName -Pattern $p -SimpleMatch -ErrorAction SilentlyContinue |
    Select-Object Path, LineNumber, Line |
    Format-Table -AutoSize |
    Out-String -Width 500 |
    Add-Content $out
}

"`n=== wsh help ===" | Add-Content $out
.\wsh.exe --help 2>&1 | Out-String -Width 300 | Add-Content $out

Write-Host "WROTE:" -ForegroundColor Green
Write-Host $out -ForegroundColor Yellow
notepad $out
