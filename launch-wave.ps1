$env:WAVETERM_NOCONFIRMQUIT = "1"
$env:WCLOUD_ENDPOINT = "https://api.waveterm.dev/central"
$env:WCLOUD_WS_ENDPOINT = "wss://wsapi.waveterm.dev"
$env:WCLOUD_PING_ENDPOINT = "https://ping.waveterm.dev/central"
$env:WAVETERM_ENVFILE = "S:\waveterm\.env"
Set-Location S:\waveterm
npm run start
