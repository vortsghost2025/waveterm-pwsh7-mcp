Using module ..\wave-coordinator.psm1

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
