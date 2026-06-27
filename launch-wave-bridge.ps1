$env:WAVETERM_NOCONFIRMQUIT = "1"
$env:WAVETERM_INSTANCE_SUFFIX = "bridge"
$env:WCLOUD_ENDPOINT = "https://api.waveterm.dev/central"
$env:WCLOUD_WS_ENDPOINT = "wss://wsapi.waveterm.dev"
$env:WCLOUD_PING_ENDPOINT = "https://ping.waveterm.dev/central"
$env:WAVETERM_ENVFILE = "S:\waveterm\.env"

$bridgeRoot = Join-Path $env:LOCALAPPDATA "waveterm-bridge"
$sourceRoot = Join-Path $env:LOCALAPPDATA "waveterm-alt"
$env:WAVETERM_DATA_HOME = Join-Path $bridgeRoot "data"
$env:WAVETERM_CONFIG_HOME = Join-Path $bridgeRoot "config"

New-Item -ItemType Directory -Force -Path $env:WAVETERM_DATA_HOME, $env:WAVETERM_CONFIG_HOME | Out-Null

$sourceConfigDir = Join-Path $sourceRoot "config"
$seedFiles = @(
    "waveai.json",
    "settings.json",
    "profiles.json",
    "connections.json"
)

foreach ($name in $seedFiles) {
    $src = Join-Path $sourceConfigDir $name
    $dst = Join-Path $env:WAVETERM_CONFIG_HOME $name
    if ((Test-Path $src) -and -not (Test-Path $dst)) {
        Copy-Item $src $dst -Force
    }
}

Set-Location S:\waveterm
npm run start
