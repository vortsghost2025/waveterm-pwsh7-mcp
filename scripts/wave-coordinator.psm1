function Get-DefaultCoordinatorState {
  return [ordered]@{
    active_turn = "idle"
    last_wakeup = ""
    last_reply_offset_inbox = 0
    last_reply_offset_outbox = 0
    escalation_pending = $false
    started_at = (Get-Date -Format "o")
  }
}

function Load-CoordinatorState {
  param([string]$StatePath)
  if (-not (Test-Path $StatePath)) { return Get-DefaultCoordinatorState }
  try {
    $raw = Get-Content -Path $StatePath -Raw -Encoding UTF8
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
    return Get-DefaultCoordinatorState
  }
}

function Save-CoordinatorState {
  param($State, [string]$StatePath)
  $State | ConvertTo-Json -Depth 5 | Set-Content -Path $StatePath -Encoding UTF8
}

class ByteOffsetTracker {
  [string]$FilePath
  [long]$LastOffset

  ByteOffsetTracker([string]$filePath) {
    $this.FilePath = $filePath
    $this.LastOffset = 0
    if (Test-Path $filePath) {
      $item = Get-Item $filePath
      $this.LastOffset = $item.Length
    }
  }

  [long] GetOffset() { return $this.LastOffset }

  [bool] Poll() {
    if (-not (Test-Path $this.FilePath)) { return $false }
    $item = Get-Item $this.FilePath
    $current = $item.Length
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
    New-Item -ItemType Directory -Path $this.LockPath -Force | Out-Null
    return $true
  }

  [void] Release() {
    if (Test-Path $this.LockPath) {
      Remove-Item $this.LockPath -Recurse -Force
    }
  }
}
