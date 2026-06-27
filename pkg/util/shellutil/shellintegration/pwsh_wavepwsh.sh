# We source this file with -NoExit -File
$env:PATH = {{.WSHBINDIR_PWSH}} + "{{.PATHSEP}}" + $env:PATH

# Source dynamic script from wsh token
$waveterm_swaptoken_output = wsh token $env:WAVETERM_SWAPTOKEN pwsh 2>$null | Out-String
if ($waveterm_swaptoken_output -and $waveterm_swaptoken_output -ne "") {
    Invoke-Expression $waveterm_swaptoken_output
}
Remove-Variable -Name waveterm_swaptoken_output
Remove-Item Env:WAVETERM_SWAPTOKEN

# Load Wave completions
wsh completion powershell | Out-String | Invoke-Expression

if ($PSVersionTable.PSVersion.Major -lt 7) {
    return  # skip OSC setup entirely
}

if ($PSStyle.FileInfo.Directory -eq "`e[44;1m") {
    $PSStyle.FileInfo.Directory = "`e[34;1m"
}

$Global:_WAVETERM_SI_FIRSTPROMPT = $true
$Global:_waveterm_si_original_psconsolehostreadline = $null

# shell integration
function Global:_waveterm_si_blocked {
    # Check if we're in tmux or screen
    return ($env:TMUX -or $env:STY -or $env:TERM -like "tmux*" -or $env:TERM -like "screen*")
}

function Global:_waveterm_si_osc {
    param(
        [string]$Command,
        [string]$Json = ""
    )
    if (_waveterm_si_blocked) { return }
    if ($Json) {
        Write-Host -NoNewline "`e]16162;$Command;$Json`a"
    } else {
        Write-Host -NoNewline "`e]16162;$Command`a"
    }
}

function Global:_waveterm_si_osc7 {
    if (_waveterm_si_blocked) { return }
    
    # Percent-encode the raw path as-is (handles UNC, drive letters, etc.)
    $encoded_pwd = [System.Uri]::EscapeDataString($PWD.Path)
    
    # OSC 7 - current directory
    Write-Host -NoNewline "`e]7;file://localhost/$encoded_pwd`a"
}

function Global:_waveterm_si_precmd {
    if (_waveterm_si_blocked) { return }
    $previousSuccess = $?
    $previousNativeExit = $LASTEXITCODE
    
    if ($Global:_WAVETERM_SI_FIRSTPROMPT) {
        $metadata = [pscustomobject]@{
            shell = "pwsh"
            shellversion = $PSVersionTable.PSVersion.ToString()
            integration = $true
        }
        _waveterm_si_osc "M" ($metadata | ConvertTo-Json -Compress)
        _waveterm_si_osc7
    } else {
        $exitCode = 0
        if (-not $previousSuccess) {
            $exitCode = 1
        } elseif ($previousNativeExit -ne 0) {
            $exitCode = $previousNativeExit
        }
        _waveterm_si_osc "D" ('{"exitcode":{0}}' -f $exitCode)
    }
    _waveterm_si_osc "A"
    $Global:_WAVETERM_SI_FIRSTPROMPT = $false
}

function Global:_waveterm_si_command_start {
    param([string]$Command)
    if (_waveterm_si_blocked) { return }
    if ([string]::IsNullOrWhiteSpace($Command)) { return }
    
    $cmdLength = [Text.Encoding]::UTF8.GetByteCount($Command)
    if ($cmdLength -gt 8192) {
        $Command = "# command too large ($cmdLength bytes)"
    }
    $cmd64 = [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($Command))
    $payload = [pscustomobject]@{ cmd64 = $cmd64 }
    _waveterm_si_osc "C" ($payload | ConvertTo-Json -Compress)
}

function Global:_waveterm_si_wrap_psconsolehostreadline {
    if (Test-Path Function:\global:PSConsoleHostReadLine) {
        $Global:_waveterm_si_original_psconsolehostreadline = $function:global:PSConsoleHostReadLine
        function Global:PSConsoleHostReadLine {
            $line = & $Global:_waveterm_si_original_psconsolehostreadline
            if ($null -ne $line -and $line -is [string]) {
                _waveterm_si_command_start $line
            }
            return $line
        }
    }
}

function Global:_waveterm_si_prompt {
    if (_waveterm_si_blocked) { return }
    _waveterm_si_precmd
}

# Add the shell integration hooks to the prompt function
if (Test-Path Function:\prompt) {
    $global:_waveterm_original_prompt = $function:prompt
    function Global:prompt {
        _waveterm_si_prompt
        & $global:_waveterm_original_prompt
    }
} else {
    function Global:prompt {
        _waveterm_si_prompt
        "PS $($executionContext.SessionState.Path.CurrentLocation)$('>' * ($nestedPromptLevel + 1)) "
    }
}

_waveterm_si_wrap_psconsolehostreadline
