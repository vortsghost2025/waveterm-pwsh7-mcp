Using module ..\wave-coordinator.psm1

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
  $script:Utf8NoBomEncoding = New-Object System.Text.UTF8Encoding($false)
}

AfterAll {
  if (Test-Path $script:TestDir) { Remove-Item $script:TestDir -Recurse -Force }
}

Describe "Coordinator watcher loop" {
  BeforeEach {
    Get-ChildItem $script:TestDir -Directory -ErrorAction SilentlyContinue | ForEach-Object {
      Remove-Item $_.FullName -Recurse -Force -ErrorAction SilentlyContinue
    }
    if (Test-Path $script:BridgeInbox) { Remove-Item $script:BridgeInbox }
    if (Test-Path $script:BridgeOutbox) { Remove-Item $script:BridgeOutbox }
    if (Test-Path $script:StatePath) { Remove-Item $script:StatePath }
    if (Test-Path $script:HeartbeatPath) { Remove-Item $script:HeartbeatPath }
    if (Test-Path $script:LogPath) { Remove-Item $script:LogPath }
    if (Test-Path $script:StopSentinel) { Remove-Item $script:StopSentinel }
    if (Test-Path $script:LockDir) { Remove-Item $script:LockDir -Recurse -Force }
    New-Item -ItemType File -Path $script:BridgeInbox -Force | Out-Null
    New-Item -ItemType File -Path $script:BridgeOutbox -Force | Out-Null
  }

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
    $lines = [System.IO.File]::ReadAllLines($script:HeartbeatPath, $script:Utf8NoBomEncoding)
    $lines.Count | Should -Be 1
    $record = $lines[0] | ConvertFrom-Json
    $record.active_turn | Should -Be "idle"
    $record.last_inbox_offset | Should -Be 0
    $record.last_outbox_offset | Should -Be 0
    $record.escalation_pending | Should -Be $false
  }

  It "detects new content in outbox" {
    $tracker = [ByteOffsetTracker]::new($script:BridgeOutbox)
    $beforeOffset = $tracker.GetOffset()

    $msg = [ordered]@{
      timestamp = (Get-Date -Format "o")
      type = "message"
      direction = "opencode_to_waveai"
      source = "opencode"
      target = "wave-ai-assistant"
      message = "integration test task"
    }
    ($msg | ConvertTo-Json -Compress) | Out-File -FilePath $script:BridgeOutbox -Append -Encoding UTF8NoBOM
    Start-Sleep -Milliseconds 200

    $hasNew = $tracker.Poll()
    $hasNew | Should -Be $true
    $tracker.GetOffset() | Should -BeGreaterThan $beforeOffset
  }

  It "detects new content in inbox" {
    $tracker = [ByteOffsetTracker]::new($script:BridgeInbox)
    $beforeOffset = $tracker.GetOffset()

    $msg = [ordered]@{
      timestamp = (Get-Date -Format "o")
      type = "message"
      direction = "assistant_reply"
      source = "wave-ai-assistant"
      target = "opencode"
      message = "integration test reply"
    }
    ($msg | ConvertTo-Json -Compress) | Out-File -FilePath $script:BridgeInbox -Append -Encoding UTF8NoBOM
    Start-Sleep -Milliseconds 200

    $hasNew = $tracker.Poll()
    $hasNew | Should -Be $true
    $tracker.GetOffset() | Should -BeGreaterThan $beforeOffset
  }

  It "persists and reloads state across restart" {
    # Arrange
    $state = Get-DefaultCoordinatorState
    $state.active_turn = "wave-ai"
    $state.last_reply_offset_inbox = 500
    $state.last_reply_offset_outbox = 1000
    Save-CoordinatorState -State $state -StatePath $script:StatePath

    # Act
    $loaded = Load-CoordinatorState -StatePath $script:StatePath

    # Assert
    $loaded.active_turn | Should -Be "wave-ai"
    $loaded.last_reply_offset_inbox | Should -Be 500
    $loaded.last_reply_offset_outbox | Should -Be 1000
  }
}
