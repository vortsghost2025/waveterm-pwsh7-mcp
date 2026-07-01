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
    $raw = Get-Content -Path $StatePath -Raw -Encoding UTF8NoBOM
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
      # Swallow repair errors; return defaults anyway
    }
    return $defaults
  }
}

function Save-CoordinatorState {
  param(
    [ValidateNotNullOrEmpty()]
    [hashtable]$State,
    [string]$StatePath
  )
  $State | ConvertTo-Json -Depth 3 | Set-Content -Path $StatePath -Encoding UTF8NoBOM
}

class ByteOffsetTracker {
  [string]$FilePath
  [long]$LastOffset

  ByteOffsetTracker([string]$filePath) {
    $this.FilePath = $filePath
    if (Test-Path $filePath) {
      $item = Get-Item $filePath
      $this.LastOffset = $item.Length
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
      return $true
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
    if (Test-Path $this.LockPath) {
      Remove-Item $this.LockPath -Force
    }
  }
}
