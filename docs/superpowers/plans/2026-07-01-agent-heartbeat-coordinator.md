# Agent Heartbeat Coordinator — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a persistent PowerShell coordinator that watches bridge JSONL files, serializes agent turns, detects escalation triggers, and dispatches wakeups so OpenCode and Wave AI can collaborate without going idle after each exchange.

**Architecture:** Coordinator polls bridge files every 2s via byte-offset tracking, manages turn ownership through a directory-creation mutex on a state file, and uses `wsh ai -s -m` to wake Wave AI when its turn arrives. Escalation gates pause the loop and surface to the user.

**Tech Stack:** PowerShell 7+, existing `wsh` CLI, existing bridge JSONL files at `S:\sean-machine-janitor\bridge\`

**Spec:** `docs/superpowers/specs/2026-07-01-agent-heartbeat-coordinator-design.md`

---

## File Structure

```
S:\waveterm\scripts\
  wave-coordinator.ps1          # Main coordinator (single file, ~300 lines)
  Start-Coordinator.ps1         # Launcher (detached background process)
  Stop-Coordinator.ps1          # Graceful stopper

S:\waveterm\agent-coordination\
  coordinator-state.json        # Turn state, last offsets, escalation flag
  coordinator-heartbeat.jsonl   # 30s heartbeat records
  coordinator-log.txt           # Human-readable log
  coordinator-lock\             # Directory-creation mutex (empty dir = lock held)
  coordinator-stop              # Sentinel file (presence = request shutdown)
```

No existing files are modified. All new files are additive.

---

### Task 1: Write the failing test for state persistence

**Files:**
- Create: `S:\waveterm\scripts\Tests\wave-coordinator.Tests.ps1`

- [ ] **Step 1: Write the failing test**

Create the test file with a Pester test for coordinator state file round-trip:

```powershell
# S:\waveterm\scripts\Tests\wave-coordinator.Tests.ps1
BeforeAll {
  $script:TestDir = Join-Path $PSScriptRoot "..\agent-coordination-test"
  if (Test-Path $script:TestDir) { Remove-Item $script:TestDir -Recurse -Force }
  New-Item -ItemType Directory -Path $script:TestDir -Force | Out-Null
  $script:CoordinatorStatePath = Join-Path $script:TestDir "coordinator-state.json"
  $script:CoordinatorLogPath = Join-Path $script:TestDir "coordinator-log.txt"
  $script:CoordinatorHeartbeatPath = Join-Path $script:TestDir "coordinator-heartbeat.jsonl"
  $script:BridgeInboxPath = Join-Path $script:TestDir "bridge-inbox.jsonl"
  $script:BridgeOutboxPath = Join-Path $script:TestDir "bridge-outbox.jsonl"
  $script:StopSentinelPath = Join-Path $script:TestDir "coordinator-stop"
  $script:LockDirPath = Join-Path $script:TestDir "coordinator-lock"

  # Create empty bridge files
  New-Item -ItemType File -Path $script:BridgeInboxPath -Force | Out-Null
  New-Item -ItemType File -Path $script:BridgeOutboxPath -Force | Out-Null
}

AfterAll {
  if (Test-Path $script:TestDir) { Remove-Item $script:TestDir -Recurse -Force }
}

Describe "CoordinatorState" {
  It "creates default state file when none exists" {
    # Arrange
    if (Test-Path $script:CoordinatorStatePath) { Remove-Item $script:CoordinatorStatePath }

    # Act
    $state = Get-DefaultCoordinatorState

    # Assert
    $state.active_turn | Should -Be "idle"
    $state.last_reply_offset_inbox | Should -Be 0
    $state.last_reply_offset_outbox | Should -Be 0
    $state.escalation_pending | Should -Be $false
  }

  It "persists and reloads state from disk" {
    # Arrange
    $testState = [ordered]@{
      active_turn = "wave-ai"
      last_wakeup = (Get-Date -Format "o")
      last_reply_offset_inbox = 1024
      last_reply_offset_outbox = 2048
      escalation_pending = $true
      started_at = (Get-Date -Format "o")
    }
    $testState | ConvertTo-Json | Set-Content -Path $script:CoordinatorStatePath -Encoding UTF8

    # Act
    $loaded = Load-CoordinatorState -StatePath $script:CoordinatorStatePath

    # Assert
    $loaded.active_turn | Should -Be "wave-ai"
    $loaded.last_reply_offset_inbox | Should -Be 1024
    $loaded.last_reply_offset_outbox | Should -Be 2048
    $loaded.escalation_pending | Should -Be $true
  }

  It "resets to defaults on corrupt state file" {
    # Arrange
    Set-Content -Path $script:CoordinatorStatePath -Value "not json{{{" -Encoding UTF8

    # Act
    $state = Load-CoordinatorState -StatePath $script:CoordinatorStatePath

    # Assert
    $state.active_turn | Should -Be "idle"
    $state.last_reply_offset_inbox | Should -Be 0
  }
}

Describe "ByteOffsetTracker" {
  It "reports zero offset for empty file" {
    $tracker = [ByteOffsetTracker]::new($script:BridgeInboxPath)
    $tracker.GetOffset() | Should -Be 0
  }

  It "detects appended content via offset change" {
    $tracker = [ByteOffsetTracker]::new($script:BridgeInboxPath)
    $tracker.GetOffset() | Should -Be 0

    "test content" | Out-File -FilePath $script:BridgeInboxPath -Append -Encoding UTF8
    Start-Sleep -Milliseconds 100
    $tracker.Poll()
    $tracker.GetOffset() | Should -BeGreaterThan 0
  }

  It "does not detect unchanged file as new write" {
    $tracker = [ByteOffsetTracker]::new($script:BridgeInboxPath)
    "static content" | Out-File -FilePath $script:BridgeInboxPath -Encoding UTF8
    Start-Sleep -Milliseconds 100
    $tracker.Poll()
    $offsetAfterWrite = $tracker.GetOffset()

    Start-Sleep -Milliseconds 100
    $tracker.Poll()
    $tracker.GetOffset() | Should -Be $offsetAfterWrite
  }
}

Describe "DirectoryMutex" {
  It "acquires lock when directory does not exist" {
    if (Test-Path $script:LockDirPath) { Remove-Item $script:LockDirPath -Recurse -Force }

    $mutex = [DirectoryMutex]::new($script:LockDirPath)
    $mutex.TryAcquire() | Should -Be $true
    Test-Path $script:LockDirPath | Should -Be $true
    $mutex.Release()
  }

  It "fails to acquire when lock already held" {
    New-Item -ItemType Directory -Path $script:LockDirPath -Force | Out-Null

    $mutex = [DirectoryMutex]::new($script:LockDirPath)
    $mutex.TryAcquire() | Should -Be $false
  }

  It "releases lock and allows re-acquire" {
    New-Item -ItemType Directory -Path $script:LockDirPath -Force | Out-Null

    $mutex1 = [DirectoryMutex]::new($script:LockDirPath)
    $mutex1.TryAcquire() | Should -Be $false

    Remove-Item $script:LockDirPath -Force
    $mutex2 = [DirectoryMutex]::new($script:LockDirPath)
    $mutex2.TryAcquire() | Should -Be $true
    $mutex2.Release()
  }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `pwsh -Command "Invoke-Pester S:\waveterm\scripts\Tests\wave-coordinator.Tests.ps1 -EnableExit"`
Expected: FAIL with "command not found" or "class not found" for each function/class under test

- [ ] **Step 3: Create minimal PowerShell module file with stubs**

```powershell
# S:\waveterm\scripts\wave-coordinator.psm1
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
  param([ordered]$State, [string]$StatePath)
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `pwsh -Command "Invoke-Pester S:\waveterm\scripts\Tests\wave-coordinator.Tests.ps1 -EnableExit"`
Expected: PASS (4 describe blocks, all tests green)

- [ ] **Step 5: Commit**

```bash
git add S:\waveterm\scripts\wave-coordinator.psm1 S:\waveterm\scripts\Tests\wave-coordinator.Tests.ps1
git commit -m "test(coordinator): add state persistence and byte-offset tracker tests"
```

---

### Task 2: Write the failing test for heartbeat and escalation detection

**Files:**
- Create: (tests appended to existing `S:\waveterm\scripts\Tests\wave-coordinator.Tests.ps1`)

- [ ] **Step 1: Write the failing test**

Append these test blocks to the existing test file:

```powershell
Describe "HeartbeatWriter" {
  It "writes a heartbeat record to JSONL" {
    $hbPath = Join-Path $script:TestDir "coordinator-heartbeat.jsonl"
    if (Test-Path $hbPath) { Remove-Item $hbPath }

    $hbWriter = [HeartbeatWriter]::new($hbPath)
    $hbWriter.Write("opencode")

    $lines = Get-Content $hbPath -Encoding UTF8
    $lines.Count | Should -Be 1
    $record = $lines[0] | ConvertFrom-Json
    $record.active_turn | Should -Be "opencode"
    $record.last_inbox_offset | Should -Be 0
  }

  It "appends heartbeat records on each call" {
    $hbPath = Join-Path $script:TestDir "coordinator-heartbeat.jsonl2"
    if (Test-Path $hbPath) { Remove-Item $hbPath }

    $hbWriter = [HeartbeatWriter]::new($hbPath)
    $hbWriter.Write("idle")
    $hbWriter.Write("wave-ai")

    $lines = Get-Content $hbPath -Encoding UTF8
    $lines.Count | Should -Be 2
    ($lines[0] | ConvertFrom-Json).active_turn | Should -Be "idle"
    ($lines[1] | ConvertFrom-Json).active_turn | Should -Be "wave-ai"
  }
}

Describe "EscalationDetector" {
  It "matches DECISION trigger" {
    $detector = [EscalationDetector]::new()
    $result = $detector.Scan("DECISION: should we use minimax or openai?")
    $result.trigger | Should -Be "DECISION"
    $result.is_soft | Should -Be $false
  }

  It "matches BLOCKED trigger" {
    $detector = [EscalationDetector]::new()
    $result = $detector.Scan("BLOCKED: wave ai not responding")
    $result.trigger | Should -Be "BLOCKED"
    $result.is_soft | Should -Be $false
  }

  It "matches SHIPMENT trigger" {
    $detector = [EscalationDetector]::new()
    $result = $detector.Scan("SHIPMENT: binary rebuilt and pushed to origin")
    $result.trigger | Should -Be "SHIPMENT"
    $result.is_soft | Should -Be $false
  }

  It "matches ASK soft trigger" {
    $detector = [EscalationDetector]::new()
    $result = $detector.Scan("ASK: which model should we use?")
    $result.trigger | Should -Be "ASK"
    $result.is_soft | Should -Be $true
  }

  It "matches trailing question mark as soft trigger" {
    $detector = [EscalationDetector]::new()
    $result = $detector.Scan("Should we proceed with approach B?")
    $result.trigger | Should -Be "QUESTION"
    $result.is_soft | Should -Be $true
  }

  It "returns no trigger for routine messages" {
    $detector = [EscalationDetector]::new()
    $result = $detector.Scan("Bridge write complete. Awaiting reply.")
    $result.trigger | Should -BeNullOrEmpty
  }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `pwsh -Command "Invoke-Pester S:\waveterm\scripts\Tests\wave-coordinator.Tests.ps1 -EnableExit"`
Expected: FAIL with "class not found" for `HeartbeatWriter` and `EscalationDetector`

- [ ] **Step 3: Add stubs to wave-coordinator.psm1**

```powershell
class HeartbeatWriter {
  [string]$HeartbeatPath
  [DateTime]$LastWrite = (Get-Date)

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
    ($record | ConvertTo-Json -Compress) | Out-File -FilePath $this.HeartbeatPath -Append -Encoding UTF8
    $this.LastWrite = Get-Date
  }

  [DateTime] GetLastWriteTime() { return $this.LastWrite }
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
    # Check for trailing question mark as soft trigger
    if ($message.Trim().EndsWith("?")) {
      return @{ trigger = "QUESTION"; is_soft = $true }
    }
    return @{ trigger = $null; is_soft = $false }
  }
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `pwsh -Command "Invoke-Pester S:\waveterm\scripts\Tests\wave-coordinator.Tests.ps1 -EnableExit"`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add S:\waveterm\scripts\wave-coordinator.psm1 S:\waveterm\scripts\Tests\wave-coordinator.Tests.ps1
git commit -m "test(coordinator): add heartbeat writer and escalation detector tests"
```

---

### Task 3: Write the failing test for the watcher loop integration

**Files:**
- Create: `S:\waveterm\scripts\Tests\wave-coordinator.integration.Tests.ps1`

- [ ] **Step 1: Write the failing integration test**

```powershell
# S:\waveterm\scripts\Tests\wave-coordinator.integration.Tests.ps1
BeforeAll {
  $script:TestDir = Join-Path $PSScriptRoot "..\agent-coordination-test-integration"
  if (Test-Path $script:TestDir) { Remove-Item $script:TestDir -Recurse -Force }
  New-Item -ItemType Directory -Path $script:TestDir -Force | Out-Null

  $script:BridgeInbox = Join-Path $script:TestDir "wave-inbox.jsonl"
  $script:BridgeOutbox = Join-Path $script:TestDir "wave-outbox.jsonl"
  $script:StatePath = Join-Path $script:TestDir "coordinator-state.json"
  $script:HeartbeatPath = Join-Path $script:TestDir "coordinator-heartbeat.jsonl"
  $script:LogPath = Join-Path $script:TestDir "coordinator-log.txt"
  $script:StopSentinel = Join-Path $script:TestDir "coordinator-stop"
  $script:LockDir = Join-Path $script:TestDir "coordinator-lock"

  New-Item -ItemType File -Path $script:BridgeInbox -Force | Out-Null
  New-Item -ItemType File -Path $script:BridgeOutbox -Force | Out-Null
}

AfterAll {
  if (Test-Path $script:TestDir) { Remove-Item $script:TestDir -Recurse -Force }
}

Describe "Coordinator watcher loop" {
  It "writes heartbeat every 30s" {
    # Arrange
    $state = Get-DefaultCoordinatorState
    Save-CoordinatorState -State $state -StatePath $script:StatePath
    $hbWriter = [HeartbeatWriter]::new($script:HeartbeatPath)
    $inboxTracker = [ByteOffsetTracker]::new($script:BridgeInbox)
    $outboxTracker = [ByteOffsetTracker]::new($script:BridgeOutbox)

    # Act - write one heartbeat
    $hbWriter.Write("idle", $inboxTracker.GetOffset(), $outboxTracker.GetOffset(), $false)

    # Assert
    $lines = Get-Content $script:HeartbeatPath -Encoding UTF8
    $lines.Count | Should -Be 1
  }

  It "detects new content in outbox" {
    $tracker = [ByteOffsetTracker]::new($script:BridgeOutbox)
    $beforeOffset = $tracker.GetOffset()

    '{ "timestamp": "' + (Get-Date -Format "o") + '", "type": "message", "direction": "opencode_to_waveai", "message": "test" }' | Out-File -FilePath $script:BridgeOutbox -Append -Encoding UTF8
    Start-Sleep -Milliseconds 200

    $hasNew = $tracker.Poll()
    $hasNew | Should -Be $true
    $tracker.GetOffset() | Should -BeGreaterThan $beforeOffset
  }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `pwsh -Command "Invoke-Pester S:\waveterm\scripts\Tests\wave-coordinator.integration.Tests.ps1 -EnableExit"`
Expected: FAIL with "function not found" for Save-CoordinatorState or similar

- [ ] **Step 3: Import module in test file and run again**

Add `Import-Module (Join-Path $PSScriptRoot "..\wave-coordinator.psm1")` at top of integration test file. Run again. Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add S:\waveterm\scripts\Tests\wave-coordinator.integration.Tests.ps1
git commit -m "test(coordinator): add integration tests for watcher loop"
```

---

### Task 4: Implement the main coordinator script

**Files:**
- Create: `S:\waveterm\scripts\wave-coordinator.ps1`
- Modify: `S:\waveterm\scripts\wave-coordinator.psm1` (add TurnManager and WakeupDispatcher classes)

- [ ] **Step 1: Write the failing test for TurnManager**

Append to `wave-coordinator.Tests.ps1`:

```powershell
Describe "TurnManager" {
  It "switches turn from opencode to wave-ai after outbox write" {
    $mutex = [DirectoryMutex]::new($script:LockDirPath)
    $mutex.TryAcquire() | Should -Be $true

    $state = Get-DefaultCoordinatorState
    $state.active_turn = "opencode"
    Save-CoordinatorState -State $state -StatePath $script:CoordinatorStatePath

    $tm = [TurnManager]::new($script:CoordinatorStatePath, $mutex)
    $tm.CompleteTurn("opencode", $script:BridgeOutboxPath)

    $loaded = Load-CoordinatorState -StatePath $script:CoordinatorStatePath
    $loaded.active_turn | Should -Be "wave-ai"

    $mutex.Release()
  }

  It "returns idle when no active turn owner wrote" {
    $mutex = [DirectoryMutex]::new($script:LockDirPath)
    $mutex.TryAcquire() | Should -Be $true

    $state = Get-DefaultCoordinatorState
    Save-CoordinatorState -State $state -StatePath $script:CoordinatorStatePath

    $tm = [TurnManager]::new($script:CoordinatorStatePath, $mutex)
    $tm.CompleteTurn("wave-ai", $script:BridgeInboxPath)

    $loaded = Load-CoordinatorState -StatePath $script:CoordinatorStatePath
    $loaded.active_turn | Should -Be "opencode"

    $mutex.Release()
  }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `pwsh -Command "Invoke-Pester S:\waveterm\scripts\Tests\wave-coordinator.Tests.ps1 -EnableExit"`
Expected: FAIL with "class not found" for `TurnManager`

- [ ] **Step 3: Add TurnManager and WakeupDispatcher classes to psm1**

Append to `S:\waveterm\scripts\wave-coordinator.psm1`:

```powershell
class TurnManager {
  [string]$StatePath
  [DirectoryMutex]$Mutex

  TurnManager([string]$statePath, [DirectoryMutex]$mutex) {
    $this.StatePath = $statePath
    $this.Mutex = $mutex
  }

  [void] CompleteTurn([string]$whoWrote, [string]$bridgePath) {
    $state = Load-CoordinatorState -StatePath $this.StatePath
    if ($whoWrote -eq "opencode" -and $bridgePath -like "*outbox*") {
      $state.active_turn = "wave-ai"
    } elseif ($whoWrote -eq "wave-ai" -and $bridgePath -like "*inbox*") {
      $state.active_turn = "opencode"
    } else {
      $state.active_turn = "idle"
    }
    $state.last_wakeup = ""
    Save-CoordinatorState -State $state -StatePath $this.StatePath
  }

  [string] GetActiveTurn() {
    $state = Load-CoordinatorState -StatePath $this.StatePath
    return $state.active_turn
  }
}

class WakeupDispatcher {
  [string]$WshPath = "wsh"

  [int] WakeWaveAI([string]$message) {
    $escaped = $message -replace '"', '`"'
    $proc = Start-Process -FilePath $this.WshPath -ArgumentList "ai","-s","-m","`"$escaped`"" -NoNewWindow -Wait -PassThru -RedirectStandardError "nul"
    return $proc.ExitCode
  }

  [int] WakeOpenCode([string]$message) {
    # OpenCode is in a continuous turn; no wakeup needed.
    # This method exists for symmetry and logging.
    Write-Host "OpenCode wakeup (no-op, continuous turn): $message"
    return 0
  }
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `pwsh -Command "Invoke-Pester S:\waveterm\scripts\Tests\wave-coordinator.Tests.ps1 -EnableExit"`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add S:\waveterm\scripts\wave-coordinator.psm1 S:\waveterm\scripts\Tests\wave-coordinator.Tests.ps1
git commit -m "feat(coordinator): add TurnManager and WakeupDispatcher classes"
```

---

### Task 5: Implement the main coordinator loop in wave-coordinator.ps1

**Files:**
- Create: `S:\waveterm\scripts\wave-coordinator.ps1`

- [ ] **Step 1: Write the coordinator script**

```powershell
# S:\waveterm\scripts\wave-coordinator.ps1
# Agent Heartbeat Coordinator — watches bridge files and dispatches wakeups
# Usage: pwsh wave-coordinator.ps1 [--bridge-dir <path>] [--poll-interval <seconds>]
# Stop: creates the coordinator-stop sentinel file in the bridge dir

param(
  [string]$BridgeDir = "S:\sean-machine-janitor\bridge",
  [int]$PollInterval = 2,
  [int]$HeartbeatInterval = 30,
  [int]$ReplyTimeout = 90,
  [int]$StaleThreshold = 90
)

Import-Module (Join-Path $PSScriptRoot "wave-coordinator.psm1") -Force

$ErrorActionPreference = "Stop"

$inboxPath = Join-Path $BridgeDir "wave-inbox.jsonl"
$outboxPath = Join-Path $BridgeDir "wave-outbox.jsonl"
$statePath = Join-Path $PSScriptRoot "agent-coordination\coordinator-state.json"
$heartbeatPath = Join-Path $PSScriptRoot "agent-coordination\coordinator-heartbeat.jsonl"
$logPath = Join-Path $PSScriptRoot "agent-coordination\coordinator-log.txt"
$stopSentinel = Join-Path $PSScriptRoot "agent-coordination\coordinator-stop"
$lockDir = Join-Path $PSScriptRoot "agent-coordination\coordinator-lock"

# Ensure dirs exist
$agentCoordinationDir = Join-Path $PSScriptRoot "agent-coordination"
if (-not (Test-Path $agentCoordinationDir)) {
  New-Item -ItemType Directory -Path $agentCoordinationDir -Force | Out-Null
}

# Acquire mutex
$mutex = [DirectoryMutex]::new($lockDir)
if (-not $mutex.TryAcquire()) {
  Write-Host "Coordinator already running (lock held at $lockDir). Exiting."
  exit 1
}

# Ensure stop sentinel is removed on entry
if (Test-Path $stopSentinel) { Remove-Item $stopSentinel -Force }

# Load or create state
$state = Load-CoordinatorState -StatePath $statePath

# Initialize trackers
$inboxTracker = [ByteOffsetTracker]::new($inboxPath)
$outboxTracker = [ByteOffsetTracker]::new($outboxPath)
$hbWriter = [HeartbeatWriter]::new($heartbeatPath)
$turnManager = [TurnManager]::new($statePath, $mutex)
$wakeupDispatcher = [WakeupDispatcher]::new()
$escalationDetector = [EscalationDetector]::new()

$lastHeartbeat = Get-Date
$lastPingPongCount = 0
$currentPingPongTask = ""
$apiErrors = @()
$startTime = Get-Date

function Write-Log([string]$msg) {
  $ts = Get-Date -Format "yyyy-MM-dd HH:mm:ss"
  "$ts $msg" | Out-File -FilePath $logPath -Append -Encoding UTF8
}

function Check-StopSentinel() {
  return (Test-Path $stopSentinel)
}

function Check-StaleCoordinator() {
  if (Test-Path $heartbeatPath) {
    $lastHb = Get-Item $heartbeatPath
    if ((Get-Date) - $lastHb.LastWriteTime).TotalSeconds -gt $StaleThreshold) {
      return $true
    }
  }
  return $false
}

function Invoke-Escalation([string]$trigger, [string]$message) {
  Write-Log "ESCALATION: trigger=$trigger message=$message"
  $state.escalation_pending = $true
  $state.active_turn = "idle"
  Save-CoordinatorState -State $state -StatePath $statePath

  # Write escalation record to outbox
  $escalationRecord = [ordered]@{
    timestamp = (Get-Date -Format "o")
    type = "escalation"
    trigger = $trigger
    message = $message
    source = "coordinator"
  }
  ($escalationRecord | ConvertTo-Json -Compress) | Out-File -FilePath $outboxPath -Append -Encoding UTF8

  $hbWriter.Write("idle", $inboxTracker.GetOffset(), $outboxTracker.GetOffset(), $true)
  $lastHeartbeat = Get-Date
}

function Update-PingPong([string]$taskFingerprint) {
  if ($taskFingerprint -ne $currentPingPongTask) {
    $currentPingPongTask = $taskFingerprint
    $lastPingPongCount = 1
  } else {
    $lastPingPongCount++
    if ($lastPingPongCount -gt 4) {
      Invoke-Escalation "PING_PONG_LIMIT" "Task has ping-ponged $lastPingPongCount rounds without converging: $taskFingerprint"
    }
  }
}

Write-Log "Coordinator starting. BridgeDir=$BridgeDir PollInterval=${PollInterval}s"

try {
  while (-not (Check-StopSentinel)) {
    # Check if coordinator itself is stale (shouldn't happen, but safety net)
    if (Check-StaleCoordinator) {
      Write-Log "WARNING: Coordinator heartbeat is stale. This instance may be orphaned."
    }

    # Check for new content in bridge files
    $inboxHasNew = $inboxTracker.Poll()
    $outboxHasNew = $outboxTracker.Poll()

    if ($outboxHasNew) {
      # OpenCode wrote to outbox — it's their turn, they're already active
      $turnManager.CompleteTurn("opencode", $outboxPath)
      $state = Load-CoordinatorState -StatePath $statePath
      $state.active_turn = "wave-ai"
      Save-CoordinatorState -State $state -StatePath $statePath
      Write-Log "Detected outbox write. Turn -> wave-ai"
    }

    if ($inboxHasNew) {
      # Wave AI wrote to inbox — it's their turn, they're already active
      $turnManager.CompleteTurn("wave-ai", $inboxPath)
      $state = Load-CoordinatorState -StatePath $statePath
      $state.active_turn = "opencode"
      Save-CoordinatorState -State $state -StatePath $statePath
      Write-Log "Detected inbox write. Turn -> opencode"
    }

    # Check if Wave AI's turn is overdue (replied in chat only)
    $state = Load-CoordinatorState -StatePath $statePath
    if ($state.active_turn -eq "wave-ai" -and $state.last_wakeup) {
      $lastWakeupTime = [DateTime]::Parse($state.last_wakeup)
      $elapsed = (Get-Date) - $lastWakeupTime
      if ($elapsed.TotalSeconds -gt $ReplyTimeout) {
        Write-Log "Wave AI reply timeout (${ReplyTimeout}s). Sending nudge."
        $wakeupDispatcher.WakeWaveAI("TURN_POKE v1`nYour turn. Read bridge outbox and reply via bridge_write_reply. You did not reply via bridge in the last ${ReplyTimeout}s.") | Out-Null
        $state.last_wakeup = (Get-Date -Format "o")
        Save-CoordinatorState -State $state -StatePath $statePath
      }
    }

    # If turn is wave-ai and no wakeup has been sent yet, send one
    $state = Load-CoordinatorState -StatePath $statePath
    if ($state.active_turn -eq "wave-ai" -and -not $state.last_wakeup) {
      Write-Log "Wave AI turn active. Sending wakeup."
      $exitCode = $wakeupDispatcher.WakeWaveAI("TURN_POKE v1`nYour turn. Read bridge outbox and reply via bridge_write_reply.")
      if ($exitCode -ne 0) {
        Write-Log "WARNING: wsh ai wakeup returned exit code $exitCode"
        $apiErrors += (Get-Date)
        if ($apiErrors.Count -ge 3) {
          Invoke-Escalation "WSH_UNAVAILABLE" "wsh ai returned non-zero exit $exitCode for 3 consecutive wakeups."
        }
      } else {
        $apiErrors = @()
      }
      $state.last_wakeup = (Get-Date -Format "o")
      Save-CoordinatorState -State $state -StatePath $statePath
    }

    # Write heartbeat
    if (((Get-Date) - $lastHeartbeat).TotalSeconds -ge $HeartbeatInterval) {
      $state = Load-CoordinatorState -StatePath $statePath
      $hbWriter.Write($state.active_turn, $inboxTracker.GetOffset(), $outboxTracker.GetOffset(), $state.escalation_pending)
      $lastHeartbeat = Get-Date
    }

    Start-Sleep -Seconds $PollInterval
  }

  # Graceful shutdown
  Write-Log "Coordinator stopping (stop sentinel detected)."
  $state = Load-CoordinatorState -StatePath $statePath
  $hbWriter.Write($state.active_turn, $inboxTracker.GetOffset(), $outboxTracker.GetOffset(), $state.escalation_pending)
  $mutex.Release()
  if (Test-Path $stopSentinel) { Remove-Item $stopSentinel -Force }
  Write-Log "Coordinator stopped cleanly."
  exit 0
} catch {
  Write-Log "FATAL: $($_.Exception.Message)"
  $mutex.Release()
  if (Test-Path $stopSentinel) { Remove-Item $stopSentinel -Force }
  exit 1
}
```

- [ ] **Step 2: Run Pester tests to verify nothing broke**

Run: `pwsh -Command "Invoke-Pester S:\waveterm\scripts\Tests\wave-coordinator.Tests.ps1 -EnableExit"`
Expected: PASS (all existing tests still pass)

- [ ] **Step 3: Commit**

```bash
git add S:\waveterm\scripts\wave-coordinator.ps1 S:\waveterm\scripts\wave-coordinator.psm1
git commit -m "feat(coordinator): implement main watcher loop with turn management and escalation"
```

---

### Task 6: Write Start-Coordinator.ps1 and Stop-Coordinator.ps1

**Files:**
- Create: `S:\waveterm\scripts\Start-Coordinator.ps1`
- Create: `S:\waveterm\scripts\Stop-Coordinator.ps1`

- [ ] **Step 1: Write Start-Coordinator.ps1**

```powershell
# S:\waveterm\scripts\Start-Coordinator.ps1
param(
  [string]$BridgeDir = "S:\sean-machine-janitor\bridge",
  [int]$PollInterval = 2
)

$coordinatorScript = Join-Path $PSScriptRoot "wave-coordinator.ps1"
$stopSentinel = Join-Path $PSScriptRoot "agent-coordination\coordinator-stop"

# Clean stop sentinel from previous run
if (Test-Path $stopSentinel) { Remove-Item $stopSentinel -Force }

# Check if already running via lock dir
$lockDir = Join-Path $PSScriptRoot "agent-coordination\coordinator-lock"
if (Test-Path $lockDir) {
  Write-Host "Coordinator already running (lock exists at $lockDir). Use Stop-Coordinator.ps1 first."
  exit 1
}

$args = "-NoProfile -ExecutionPolicy Bypass -File `"$coordinatorScript`" --bridge-dir `"$BridgeDir`" --poll-interval $PollInterval"

$proc = Start-Process -FilePath "pwsh" -ArgumentList $args -WindowStyle Hidden -PassThru
Start-Sleep -Seconds 2

if (-not $proc.HasExited) {
  Write-Host "Coordinator started. PID=$($proc.Id)"
  Write-Host "BridgeDir: $BridgeDir"
  Write-Host "Log: $(Join-Path $PSScriptRoot agent-coordination\coordinator-log.txt)"
  Write-Host "Stop with: $(Join-Path $PSScriptRoot Stop-Coordinator.ps1)"
} else {
  Write-Host "Coordinator exited immediately. Check log: $(Join-Path $PSScriptRoot agent-coordination\coordinator-log.txt)"
  exit $proc.ExitCode
}
```

- [ ] **Step 2: Write Stop-Coordinator.ps1**

```powershell
# S:\waveterm\scripts\Stop-Coordinator.ps1
$stopSentinel = Join-Path $PSScriptRoot "agent-coordination\coordinator-stop"
$logPath = Join-Path $PSScriptRoot "agent-coordination\coordinator-log.txt"
$lockDir = Join-Path $PSScriptRoot "agent-coordination\coordinator-lock"

if (-not (Test-Path $lockDir)) {
  Write-Host "Coordinator is not running (no lock at $lockDir)."
  exit 0
}

Write-Host "Sending stop signal to coordinator..."
New-Item -ItemType File -Path $stopSentinel -Force | Out-Null

# Wait for lock to be released (coordinator cleans it up on shutdown)
$timeout = 15
$elapsed = 0
while ((Test-Path $lockDir) -and $elapsed -lt $timeout) {
  Start-Sleep -Seconds 1
  $elapsed++
}

if (Test-Path $lockDir) {
  Write-Host "Coordinator did not stop within ${timeout}s. Force-killing..."
  # Find coordinator process by checking who holds the lock (PS process with wave-coordinator.ps1 in cmdline)
  $procs = Get-CimInstance Win32_Process -Filter "Name = 'pwsh.exe'" | Where-Object {
    $_.CommandLine -like "*wave-coordinator.ps1*"
  }
  foreach ($p in $procs) {
    Write-Host "Killing PID $($p.ProcessId)"
    Stop-Process -Id $p.ProcessId -Force
  }
  Start-Sleep -Seconds 2
  if (Test-Path $lockDir) {
    Write-Host "WARNING: Lock dir still exists. Manual cleanup may be needed at $lockDir"
  }
} else {
  Write-Host "Coordinator stopped cleanly."
}

Write-Host "Log available at: $logPath"
```

- [ ] **Step 3: Test Start/Stop cycle manually**

```powershell
# Start
pwsh S:\waveterm\scripts\Start-Coordinator.ps1
# Verify running
Test-Path "S:\waveterm\agent-coordination\coordinator-lock"
# Verify heartbeat
Start-Sleep -Seconds 35
Get-Content "S:\waveterm\agent-coordination\coordinator-heartbeat.jsonl" -Tail 1
# Stop
pwsh S:\waveterm\scripts\Stop-Coordinator.ps1
# Verify stopped
Test-Path "S:\waveterm\agent-coordination\coordinator-lock"  # should be False
```

Expected: Start reports PID, heartbeat appears after 30s, stop reports clean shutdown, lock dir removed.

- [ ] **Step 4: Commit**

```bash
git add S:\waveterm\scripts\Start-Coordinator.ps1 S:\waveterm\scripts\Stop-Coordinator.ps1
git commit -m "feat(coordinator): add Start-Coordinator and Stop-Coordinator launchers"
```

---

### Task 7: End-to-end integration test with real bridge files

**Files:**
- Modify: `S:\waveterm\scripts\Tests\wave-coordinator.integration.Tests.ps1`

- [ ] **Step 1: Write the end-to-end test**

```powershell
Describe "End-to-end coordinator with live bridge" {
  It "completes a full turn cycle: opencode writes, coordinator wakes wave-ai, wave-ai replies, coordinator passes back" {
    # Arrange
    $state = Get-DefaultCoordinatorState
    $state.active_turn = "idle"
    Save-CoordinatorState -State $state -StatePath $script:CoordinatorStatePath

    # Simulate OpenCode writing to outbox
    $msg1 = [ordered]@{
      timestamp = (Get-Date -Format "o")
      type = "message"
      direction = "opencode_to_waveai"
      source = "opencode"
      target = "wave-ai-assistant"
      message = "E2E test task: please confirm receipt"
    }
    ($msg1 | ConvertTo-Json -Compress) | Out-File -FilePath $script:BridgeOutboxPath -Append -Encoding UTF8

    # Start coordinator in background
    $coordinatorScript = Join-Path $PSScriptRoot "..\wave-coordinator.ps1"
    $proc = Start-Process -FilePath "pwsh" -ArgumentList "-NoProfile","-File",$coordinatorScript,"--bridge-dir",$script:TestDir,"--poll-interval","1","--heartbeat-interval","5" -NoNewWindow -PassThru -RedirectStandardOutput "$($script:TestDir)\coordinator-stdout.log" -RedirectStandardError "$($script:TestDir)\coordinator-stderr.log"

    try {
      # Wait for coordinator to detect outbox write and set turn to wave-ai
      $maxWait = 10
      $waited = 0
      while ($waited -lt $maxWait) {
        $state = Load-CoordinatorState -StatePath $script:CoordinatorStatePath
        if ($state.active_turn -eq "wave-ai" -and $state.last_wakeup) { break }
        Start-Sleep -Seconds 1
        $waited++
      }
      $state.active_turn | Should -Be "wave-ai"

      # Simulate Wave AI replying via bridge inbox
      $msg2 = [ordered]@{
        timestamp = (Get-Date -Format "o")
        type = "message"
        direction = "assistant_reply"
        source = "wave-ai-assistant"
        target = "opencode"
        message = "E2E test reply: receipt confirmed"
      }
      ($msg2 | ConvertTo-Json -Compress) | Out-File -FilePath $script:BridgeInboxPath -Append -Encoding UTF8

      # Wait for coordinator to detect inbox write and set turn back to opencode
      $waited = 0
      while ($waited -lt $maxWait) {
        $state = Load-CoordinatorState -StatePath $script:CoordinatorStatePath
        if ($state.active_turn -eq "opencode") { break }
        Start-Sleep -Seconds 1
        $waited++
      }
      $state.active_turn | Should -Be "opencode"
    } finally {
      # Stop coordinator
      $stopSentinel = Join-Path $script:TestDir "coordinator-stop"
      New-Item -ItemType File -Path $stopSentinel -Force | Out-Null
      $proc.WaitForExit(5000) | Out-Null
      if (-not $proc.HasExited) {
        Stop-Process -Id $proc.Id -Force
      }
    }
  }
}
```

- [ ] **Step 2: Run test to verify it passes**

Run: `pwsh -Command "Invoke-Pester S:\waveterm\scripts\Tests\wave-coordinator.integration.Tests.ps1 -EnableExit"`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add S:\waveterm\scripts\Tests\wave-coordinator.integration.Tests.ps1
git commit -m "test(coordinator): add end-to-end turn cycle integration test"
```

---

### Task 8: Wire coordinator into existing agent protocol

**Files:**
- Modify: `S:\waveterm\AGENTS_PROTOCOL.md`

- [ ] **Step 1: Update AGENTS_PROTOCOL.md Wakeup section**

Replace the "Wakeup (the bootstrap problem)" section (lines 44-70) with:

```markdown
## Wakeup (the bootstrap problem)

The coordinator (`scripts/wave-coordinator.ps1`) watches both bridge files
and dispatches wakeups automatically. Agents no longer need to manually call
`wsh ai -s -m` after writing to the bridge. The coordinator handles turn
transitions.

To start the coordinator: run `scripts/Start-Coordinator.ps1`.
To stop it: run `scripts/Stop-Coordinator.ps1`.

If the coordinator is not running, fall back to the manual wakeup:
`wsh ai -s -m "TURN_POKE v1\nYour turn. Read bridge outbox and reply via bridge_write_reply.\n"`
```

- [ ] **Step 2: Commit**

```bash
git add S:\waveterm\AGENTS_PROTOCOL.md
git commit -m "docs: update AGENTS_PROTOCOL.md to use coordinator instead of manual wakeup"
```

---

### Task 9: Run full test suite and verify binaries are unaffected

- [ ] **Step 1: Run all coordinator tests**

```powershell
pwsh -Command "Invoke-Pester S:\waveterm\scripts\Tests\wave-coordinator.Tests.ps1 S:\waveterm\scripts\Tests\wave-coordinator.integration.Tests.ps1 -EnableExit"
```

Expected: All PASS

- [ ] **Step 2: Verify Go tests still pass**

```powershell
cd S:\waveterm
go test ./pkg/aiusechat/... -count=1
go test ./cmd/wsh/cmd/... -count=1
```

Expected: All PASS (coordinator is additive PowerShell, no Go changes)

- [ ] **Step 3: Commit any final fixes**

```bash
git add -A
git commit -m "test(coordinator): run full coordinator test suite and verify no regressions"
```

---

## Self-Review

After writing all tasks, verify:

1. **Spec coverage:** Every section in the design spec has at least one implementing task.
2. **No placeholders:** All code blocks contain complete, working code.
3. **Type consistency:** Method signatures match across all tasks.
4. **Test completeness:** Every public class/function has a corresponding Pester test.
