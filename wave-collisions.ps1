# wave-collisions.ps1
# Observational report of every kilo/opencode/wave-related process on this box.
#
# NOT a kill tool. NOT a config mutator. Read-only from PID perspective.
# Side-effects: nothing outside this script's read.
#
# Outputs: stdout table. No secrets printed.
#
# Usage:
#   pwsh -NoLogo -NoProfile -File wave-collisions.ps1
#   pwsh -NoLogo -NoProfile -File wave-collisions.ps1 -StaleHours 4
#   pwsh -NoLogo -NoProfile -File wave-collisions.ps1 -Json
#
[CmdletBinding()]
param(
    [int]$StaleHours = 4,
    [int]$RecentTouchMinutes = 30,
    [switch]$Json
)

$ErrorActionPreference = 'Stop'

# --- Process discovery -------------------------------------------------------
# We do NOT match by exe-name only; we also match by command-line patterns
# that uniquely identify agent runtimes, to catch renamed / wrapper invocations.
$agentPatterns = @(
    @{ Kind = 'kilo';     Match = 'kilo\.exe$' },
    @{ Kind = 'opencode'; Match = 'opencode(\.exe|-shim)?$' },
    @{ Kind = 'kilocode'; Match = 'kilocode' },
    @{ Kind = 'wavesrv';  Match = 'wavesrv\.x64(\.exe)?$' },
    @{ Kind = 'wave-mcp'; Match = 'wave-mcp(\.exe)?$' },
    @{ Kind = 'wsh';      Match = '^wsh(\.exe)?$' }
)

function Get-IsAgentCommandLine {
    param([string]$cmd)
    if (-not $cmd) { return $false }
    # Only on cmdlines that *invoke* an agent runtime as the main executable or
    # argument target. Process name alone is too noisy (Notepad editing
    # workspace-kilocode/README.md matched earlier).
    if ($cmd -match '\b(kilo|opencode|wavesrv\.x64|wave-mcp|wsh)\.exe\b') { return $true }
    if ($cmd -match '@kilocode/cli\b') { return $true }
    if ($cmd -match '\bscoop[/\\]shims[/\\]opencode\b') { return $true }
    return $false
}

function Get-AgentProcesses {
    $procs = Get-CimInstance Win32_Process -ErrorAction SilentlyContinue
    $out = foreach ($p in $procs) {
        $kind = $null
        $exeName = ''
        try { $exeName = [System.IO.Path]::GetFileName($p.ExecutablePath) } catch {}

        $cmdIsAgent = Get-IsAgentCommandLine $p.CommandLine

        switch -regex ($exeName) {
            '^kilo\.exe$'                  { $kind = 'kilo' }
            '^node\.exe$'                  {
                # node-bundled kilos wrap `kilo` as `node .../kilo serve`
                if ($p.CommandLine -match '@kilocode' -or $p.CommandLine -match '\bkilo\s') { $kind = 'kilo' }
            }
            '^opencode(\.exe|-shim)?$'     { $kind = 'opencode' }
            '^kilocode(\.exe)?$'           { $kind = 'kilocode' }
            '^wavesrv\.x64(\.exe)?$'       { $kind = 'wavesrv' }
            '^wave-mcp(\.exe)?$'           { $kind = 'wave-mcp' }
            '^wsh(\.exe)?$'                { $kind = 'wsh' }
        }

        if (-not $kind -or -not $cmdIsAgent) { continue }

        [pscustomobject]@{
            Kind          = $kind
            Pid           = $p.ProcessId
            ParentPid     = $p.ParentProcessId
            StartedAt     = $p.CreationDate
            ExePath       = $p.ExecutablePath
            CommandLine   = $p.CommandLine
        }
    }
    return $out
}

# --- Profile inference -------------------------------------------------------
# Best-effort: pull profile from command line (-ProfileName kilo-b), paths
# pointing into %LOCALAPPDATA%\AgentProfiles\<name>\, or HOME-equivalents.
$profilesRoot = Join-Path $env:LOCALAPPDATA 'AgentProfiles'

function Get-AncestorPid($proc_pid, [int]$max = 8) {
    $p = $proc_pid
    for ($i = 0; $i -lt $max; $i++) {
        $proc = Get-CimInstance Win32_Process -Filter "ProcessId=$p" -ErrorAction SilentlyContinue
        if (-not $proc) { return $null }
        if ($proc.ParentProcessId -eq 0 -or $proc.ParentProcessId -eq $proc.ProcessId) { return $null }
        $p = $proc.ParentProcessId
    }
    return $p
}

function Infer-Profile($proc, $alreadyKnown) {
    if ($alreadyKnown) { return $alreadyKnown }

    # 1. Direct command-line hint (kilo-b.ps1 / opencode direct).
    if ($proc.CommandLine) {
        $m = [regex]::Match($proc.CommandLine, '-ProfileName\s+([\w-]+)')
        if ($m.Success) { return $m.Groups[1].Value }
    }
    # 2. PATH hint inside exe path.
    if ($proc.ExePath) {
        $m = [regex]::Match($proc.ExePath, '\\AgentProfiles\\([^\\]+)\\', 'IgnoreCase')
        if ($m.Success) { return $m.Groups[1].Value }
    }
    # 3. Walk up the parent chain looking for the launcher wrapper.
    $cur = $proc.ParentPid
    for ($i = 0; $i -lt 8 -and $cur -ne 0 -and $cur -ne $null; $i++) {
        $parent = Get-CimInstance Win32_Process -Filter "ProcessId=$cur" -ErrorAction SilentlyContinue
        if (-not $parent) { break }
        if ($parent.CommandLine) {
            $m = [regex]::Match($parent.CommandLine, '-ProfileName\s+([\w-]+)')
            if ($m.Success) { return $m.Groups[1].Value }
        }
        $cur = $parent.ParentProcessId
    }
    return $null
}

# --- Recent-config-touches ---------------------------------------------------
# Files in known profile dirs written within the window. NO env values printed.
$touchRoots = @(
    @{ Path = (Join-Path $env:LOCALAPPDATA 'AgentProfiles');         Mask = '\\AgentProfiles\\' }
    @{ Path = (Join-Path $env:LOCALAPPDATA 'waveterm-alt');           Mask = '\\waveterm-alt\' }
    @{ Path = (Join-Path $env:LOCALAPPDATA 'waveterm-bridge');        Mask = '\\waveterm-bridge\' }
    @{ Path = (Join-Path $env:LOCALAPPDATA 'opencode');               Mask = '\\opencode\' }
    @{ Path = (Join-Path $env:USERPROFILE '.config\opencode');        Mask = '\\.config\opencode\' }
    @{ Path = (Join-Path $env:USERPROFILE '.local\share\opencode');   Mask = '\\.local\share\opencode\' }
    @{ Path = (Join-Path $env:USERPROFILE '.local\share\kilo');       Mask = '\\.local\share\kilo\' }
)

# "Config-y" files. Skip snapshot object internals and DB write-ahead logs to
# avoid noise. We want to see CONFIG files, agents/*.go changes, secret-file
# mtimes — not the opencode DB churn.
$touchIncludePatterns = @(
    '*.json*', '*.yaml', '*.yml', '*.toml',
    '*.md', '*.ps1', '*.sh', '*.go',
    '*.lock', '*.pid'
)
$touchExcludeSubstrings = @(
    '\snapshot\',           # opencode git snapshot object store
    '\.git\',
    '\session_diff\',
    '\node_modules\'
)

function Get-RecentTouches($root, $minutes) {
    if (-not (Test-Path -LiteralPath $root)) { return @() }
    $cutoff = (Get-Date).AddMinutes(-1 * $minutes)
    Get-ChildItem -LiteralPath $root -Recurse -File -Force -ErrorAction SilentlyContinue |
        Where-Object { $_.LastWriteTime -ge $cutoff -and
            ((Get-Item -LiteralPath $_.FullName).Extension -in '.json','.yaml','.yml','.toml','.md','.ps1','.sh','.go','.lock','.pid','' -or $_.Name -match '^\..*$|kilo\.jsonc|opencode\.json$|secrets\.ps1$') } |
        Where-Object {
            $full = $_.FullName
            -not ($touchExcludeSubstrings | Where-Object { $full -like "*$_*" } | Select-Object -First 1)
        } |
        Select-Object FullName, LastWriteTime, @{Name = 'Size'; Expression = { $_.Length } } -First 30
}

# --- Stale flag --------------------------------------------------------------
function Test-Stale($startedAt, $hours) {
    if (-not $startedAt) { return $true }
    return ((Get-Date) - [datetime]$startedAt).TotalHours -ge $hours
}

# --- Build report ------------------------------------------------------------
$procs = Get-AgentProcesses
$now = Get-Date

# Annotate each row
$rows = foreach ($p in $procs) {
    $profile = Infer-Profile $p
    $ageHrs = [math]::Round(((Get-Date) - [datetime]$p.StartedAt).TotalHours, 1)
    $isStale = Test-Stale $p.StartedAt $StaleHours
    [pscustomobject]@{
        Kind          = $p.Kind
        Pid           = $p.Pid
        ParentPid     = $p.ParentPid
        StartedAt     = $p.StartedAt
        AgeHours      = $ageHrs
        Profile       = if ($profile) { $profile } else { '-' }
        Stale         = $isStale
        ExePath       = $p.ExePath
        CommandLine   = $p.CommandLine
    }
}
$rows = $rows | Sort-Object StartedAt

# --- Touches per profile root -----------------------------------------------
$touches = foreach ($root in $touchRoots) {
    $hits = Get-RecentTouches $root.Path $RecentTouchMinutes
    foreach ($h in $hits) {
        [pscustomobject]@{
            Root      = $root.Mask
            File      = $h.FullName
            LastWrite = $h.LastWriteTime
            Size      = $h.Size
        }
    }
}

$summary = [pscustomobject]@{
    GeneratedAt     = $now.ToString('o')
    StaleHours      = $StaleHours
    RecentTouchMin  = $RecentTouchMinutes
    Agents          = $rows
    ActivePids      = ($rows | Measure-Object).Count
    StalePids       = ($rows | Where-Object Stale | Measure-Object).Count
    RecentTouches   = $touches
}

if ($Json) {
    $payload = @{
        generated_at     = $summary.GeneratedAt
        stale_threshold_hours = $summary.StaleHours
        recent_touch_minutes  = $summary.RecentTouchMin
        agents           = @($summary.Agents | ForEach-Object {
            [ordered]@{
                kind      = $_.Kind
                pid       = $_.Pid
                parent    = $_.ParentPid
                started   = if ($_.StartedAt) { $_.StartedAt.ToString('o') } else { $null }
                age_hours = $_.AgeHours
                profile   = $_.Profile
                stale     = $_.Stale
                exe_path  = $_.ExePath
                cmd_line  = $_.CommandLine
            }
        })
        recent_touches   = @($summary.RecentTouches | ForEach-Object {
            [ordered]@{
                root = $_.Root
                file = $_.File
                last_write = $_.LastWrite.ToString('o')
                size = $_.Size
            }
        })
        active_pids      = $summary.ActivePids
        stale_pids       = $summary.StalePids
    }
    $payload | ConvertTo-Json -Depth 6
    return
}

# Human-readable output. Two tables.
Write-Host "=== AGENTS (Stale >= $StaleHours h) ===" -ForegroundColor Cyan
Write-Host ("{0,-9} {1,-10} {2,-10} {3,-19} {4,-6} {5,-12} {6,-7} {7}" -f 'Kind','PID','Parent','Started','Hours','Profile','Stale','CommandLine')
Write-Host ('-' * 130)
foreach ($r in $rows) {
    $staleMarker = if ($r.Stale) { 'YES' } else { '-' }
    Write-Host ("{0,-9} {1,-10} {2,-10} {3,-19} {4,-6} {5,-12} {6,-7} {7}" -f $r.Kind,$r.Pid,$r.ParentPid,($r.StartedAt.ToString('yyyy-MM-dd HH:mm')),$r.AgeHours,$r.Profile,$staleMarker,$r.CommandLine)
}
Write-Host ''
Write-Host ("Totals: active={0} stale={1}" -f $summary.ActivePids,$summary.StalePids)
Write-Host ''
Write-Host "=== RECENT CONFIG TOUCHES (last $RecentTouchMinutes min) ===" -ForegroundColor Cyan
if (-not $touches -or $touches.Count -eq 0) {
    Write-Host '  (none)'
} else {
    foreach ($t in $touches) {
        Write-Host ("  {0}  {1,10}  {2}" -f $t.LastWrite.ToString('HH:mm:ss'),$t.Size,$t.File)
    }
}
