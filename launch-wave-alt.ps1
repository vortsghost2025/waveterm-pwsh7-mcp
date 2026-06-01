$env:WAVETERM_NOCONFIRMQUIT = "1"
$env:WAVETERM_INSTANCE_SUFFIX = "alt"
$env:WCLOUD_ENDPOINT = "https://api.waveterm.dev/central"
$env:WCLOUD_WS_ENDPOINT = "wss://wsapi.waveterm.dev"
$env:WCLOUD_PING_ENDPOINT = "https://ping.waveterm.dev/central"
$env:WAVETERM_ENVFILE = "S:\waveterm\.env"

$altRoot = Join-Path $env:LOCALAPPDATA "waveterm-alt"
$env:WAVETERM_DATA_HOME = Join-Path $altRoot "data"
$env:WAVETERM_CONFIG_HOME = Join-Path $altRoot "config"

New-Item -ItemType Directory -Force -Path $env:WAVETERM_DATA_HOME, $env:WAVETERM_CONFIG_HOME | Out-Null

Set-Location S:\waveterm
npm run start
