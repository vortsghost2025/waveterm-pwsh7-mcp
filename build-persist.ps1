# Build wsh and server with chat persistence + wsh input command
Write-Host "Building wsh.exe..." -ForegroundColor Cyan
go build -o wsh.exe ./cmd/wsh
if ($LASTEXITCODE -ne 0) { Write-Host "wsh build FAILED" -ForegroundColor Red; exit 1 }
Write-Host "wsh.exe built OK" -ForegroundColor Green

Write-Host ""
Write-Host "Building server.exe..." -ForegroundColor Cyan
go build -o server.exe ./cmd/server
if ($LASTEXITCODE -ne 0) { Write-Host "server build FAILED" -ForegroundColor Red; exit 1 }
Write-Host "server.exe built OK" -ForegroundColor Green

Write-Host ""
Write-Host "Both builds succeeded!" -ForegroundColor Green
