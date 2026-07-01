function Get-DefaultCoordinatorState {
  return [ordered]@{
    active_turn = "idle"
    last_wakeup = ""
    last_reply_offset_inbox = 0
    last_reply_offset_outbox = 0
    escalation_pending = $false
  }
}

function Load-CoordinatorState {
  param(
    [ValidateNotNullOrEmpty()]
    [string]$StatePath
  )
  if (-not (Test-Path $StatePath)) {
    $state = Get-DefaultCoordinatorState
    $state.started_at = (Get-Date -Format "o")
    return $state
  }
  try {
    $raw = [System.IO.File]::ReadAllText($StatePath, $Utf8NoBom)
    $parsed = $raw | ConvertFrom-Json
    return [ordered]@{
      active_turn = $parsed.active_turn
      last_wakeup = $parsed.last_wakeup
      last_reply_offset_inbox = [long]$parsed.last_reply_offset_inbox
      last_reply_offset_outbox = [long]$parsed.last_reply_offset_outbox
      escalation_pending = [bool]$parsed.escalation_pending
      started_at = $parsed.started_at
    }
  } catch {
    $defaults = Get-DefaultCoordinatorState
    try {
      Save-CoordinatorState -State $defaults -StatePath $StatePath
    } catch {
      Write-Warning "Failed to repair coordinator state file '$StatePath': $_"
    }
    return $defaults
  }
}

$Utf8NoBom = New-Object System.Text.UTF8Encoding($false)

function Save-CoordinatorState {
  param(
    [ValidateNotNullOrEmpty()]
    [System.Collections.IDictionary]$State,
    [string]$StatePath
  )
  $json = $State | ConvertTo-Json -Depth 3
  $parent = Split-Path -Parent $StatePath
  if (-not (Test-Path $parent)) {
    New-Item -ItemType Directory -Path $parent -Force | Out-Null
  }
  $tmpPath = "$StatePath.tmp"
  [System.IO.File]::WriteAllText($tmpPath, $json, $Utf8NoBom)
  Move-Item -Path $tmpPath -Destination $StatePath -Force
}

class ByteOffsetTracker {
  [string]$FilePath
  [long]$LastOffset

  ByteOffsetTracker([string]$filePath) {
    $this.FilePath = $filePath
    if (Test-Path $filePath) {
      try {
        $item = Get-Item $filePath
        $this.LastOffset = $item.Length
      } catch {
        $this.LastOffset = 0
      }
    }
  }

  [long] GetOffset() { return $this.LastOffset }

  [bool] Poll() {
    try {
      $item = Get-Item $this.FilePath -ErrorAction Stop
    } catch {
      return $false
    }
    $current = $item.Length
    if ($current -lt $this.LastOffset) {
      $this.LastOffset = $current
      return $true  # File truncated — caller should re-read from new offset
    }
    if ($current -gt $this.LastOffset) {
      $this.LastOffset = $current
      return $true
    }
    return $false
  }
}

class DirectoryMutex {
  [string]$LockPath

  DirectoryMutex([string]$lockPath) {
    $this.LockPath = $lockPath
  }

  [bool] TryAcquire() {
    if (Test-Path $this.LockPath) { return $false }
    try {
      New-Item -ItemType Directory -Path $this.LockPath -ErrorAction Stop | Out-Null
      return $true
    } catch {
      return $false
    }
  }

  [void] Release() {
    if ([string]::IsNullOrWhiteSpace($this.LockPath)) { return }
    if (Test-Path $this.LockPath) {
      Remove-Item $this.LockPath -Force
    }
  }
}

class HeartbeatWriter {
  [string]$HeartbeatPath

  HeartbeatWriter([string]$heartbeatPath) {
    $this.HeartbeatPath = $heartbeatPath
  }

  [void] Write([string]$activeTurn, [long]$inboxOffset, [long]$outboxOffset, [bool]$escalationPending) {
    $record = [ordered]@{
      timestamp = (Get-Date -Format "o")
      active_turn = $activeTurn
      last_inbox_offset = $inboxOffset
      last_outbox_offset = $outboxOffset
      escalation_pending = $escalationPending
    }
    $json = ($record | ConvertTo-Json -Compress)
    [System.IO.File]::AppendAllText($this.HeartbeatPath, $json + "`n", (New-Object System.Text.UTF8Encoding($false)))
  }
}

class EscalationDetector {
  [hashtable]$Patterns = @{
    "DECISION" = "DECISION:"
    "BLOCKED" = "BLOCKED:"
    "SHIPMENT" = "SHIPMENT:"
    "ASK" = "ASK:"
  }

  EscalationDetector() {}

  [hashtable] Scan([string]$message) {
    $upper = $message.ToUpper()
    foreach ($key in $this.Patterns.Keys) {
      if ($upper.Contains($this.Patterns[$key])) {
        return @{ trigger = $key; is_soft = ($key -eq "ASK") }
      }
    }
    if ($message.Trim().EndsWith("?")) {
      return @{ trigger = "QUESTION"; is_soft = $true }
    }
    return @{ trigger = $null; is_soft = $false }
  }
}
