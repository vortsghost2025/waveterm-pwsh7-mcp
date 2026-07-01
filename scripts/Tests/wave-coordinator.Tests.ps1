Using module ..\wave-coordinator.psm1

BeforeAll {
  $script:BaseTestDir = Join-Path $PSScriptRoot "..\agent-coordination-test"
  if (Test-Path $script:BaseTestDir) { Remove-Item $script:BaseTestDir -Recurse -Force }
  New-Item -ItemType Directory -Path $script:BaseTestDir -Force | Out-Null
  $script:Utf8NoBomEncoding = New-Object System.Text.UTF8Encoding($false)
}

AfterAll {
  if (Test-Path $script:BaseTestDir) { Remove-Item $script:BaseTestDir -Recurse -Force }
}

Describe "CoordinatorState" {
  BeforeEach {
    Get-ChildItem $script:BaseTestDir -Directory -ErrorAction SilentlyContinue | ForEach-Object {
      Remove-Item $_.FullName -Recurse -Force -ErrorAction SilentlyContinue
    }
  }

  It "creates default state file when none exists" {
    $dir = Join-Path $script:BaseTestDir ([Guid]::NewGuid().ToString("N").Substring(0, 8))
    New-Item -ItemType Directory -Path $dir -Force | Out-Null
    $statePath = Join-Path $dir "coordinator-state.json"

    $state = Get-DefaultCoordinatorState

    $state.active_turn | Should -Be "idle"
    $state.last_reply_offset_inbox | Should -Be 0
    $state.last_reply_offset_outbox | Should -Be 0
    $state.escalation_pending | Should -Be $false
  }

  It "persists and reloads state from disk" {
    $dir = Join-Path $script:BaseTestDir ([Guid]::NewGuid().ToString("N").Substring(0, 8))
    New-Item -ItemType Directory -Path $dir -Force | Out-Null
    $statePath = Join-Path $dir "coordinator-state.json"
    $testState = [ordered]@{
      active_turn = "wave-ai"
      last_wakeup = (Get-Date -Format "o")
      last_reply_offset_inbox = 1024
      last_reply_offset_outbox = 2048
      escalation_pending = $true
      started_at = (Get-Date -Format "o")
    }
    $json = $testState | ConvertTo-Json
    [System.IO.File]::WriteAllText($statePath, $json, $script:Utf8NoBomEncoding)

    $loaded = Load-CoordinatorState -StatePath $statePath

    $loaded.active_turn | Should -Be "wave-ai"
    $loaded.last_reply_offset_inbox | Should -Be 1024
    $loaded.last_reply_offset_outbox | Should -Be 2048
    $loaded.escalation_pending | Should -Be $true
    $loaded.started_at.ToString("o") | Should -Be $testState.started_at
  }

  It "resets to defaults on corrupt state file" {
    $dir = Join-Path $script:BaseTestDir ([Guid]::NewGuid().ToString("N").Substring(0, 8))
    New-Item -ItemType Directory -Path $dir -Force | Out-Null
    $statePath = Join-Path $dir "coordinator-state.json"
    [System.IO.File]::WriteAllText($statePath, "not json{{{", $script:Utf8NoBomEncoding)

    $state = Load-CoordinatorState -StatePath $statePath

    $state.active_turn | Should -Be "idle"
    $state.last_reply_offset_inbox | Should -Be 0
  }

  It "saves coordinator state to disk" {
    $dir = Join-Path $script:BaseTestDir ([Guid]::NewGuid().ToString("N").Substring(0, 8))
    New-Item -ItemType Directory -Path $dir -Force | Out-Null
    $statePath = Join-Path $dir "coordinator-state.json"
    $testState = [ordered]@{
      active_turn = "wave-ai"
      last_wakeup = "2024-01-01T00:00:00Z"
      last_reply_offset_inbox = 512
      last_reply_offset_outbox = 1024
      escalation_pending = $true
      started_at = "2024-01-01T00:00:00Z"
    }

    Save-CoordinatorState -State $testState -StatePath $statePath

    Test-Path $statePath | Should -Be $true
    $content = [System.IO.File]::ReadAllText($statePath, $script:Utf8NoBomEncoding)
    $parsed = $content | ConvertFrom-Json
    $parsed.active_turn | Should -Be "wave-ai"
    $parsed.last_reply_offset_inbox | Should -Be 512
  }

  It "round-trips state through save and load" {
    $dir = Join-Path $script:BaseTestDir ([Guid]::NewGuid().ToString("N").Substring(0, 8))
    New-Item -ItemType Directory -Path $dir -Force | Out-Null
    $statePath = Join-Path $dir "coordinator-state.json"
    $testState = [ordered]@{
      active_turn = "test-turn"
      last_wakeup = "2024-06-01T12:00:00.0000000Z"
      last_reply_offset_inbox = 333
      last_reply_offset_outbox = 777
      escalation_pending = $false
      started_at = "2024-06-01T12:00:00.0000000Z"
    }

    Save-CoordinatorState -State $testState -StatePath $statePath
    $loaded = Load-CoordinatorState -StatePath $statePath

    $loaded.active_turn | Should -Be "test-turn"
    $loaded.last_wakeup.ToString("o") | Should -Be "2024-06-01T12:00:00.0000000Z"
    $loaded.last_reply_offset_inbox | Should -Be 333
    $loaded.last_reply_offset_outbox | Should -Be 777
    $loaded.escalation_pending | Should -Be $false
    $loaded.started_at.ToString("o") | Should -Be "2024-06-01T12:00:00.0000000Z"
  }
}

Describe "ByteOffsetTracker" {
  BeforeEach {
    Get-ChildItem $script:BaseTestDir -Directory -ErrorAction SilentlyContinue | ForEach-Object {
      Remove-Item $_.FullName -Recurse -Force -ErrorAction SilentlyContinue
    }
  }

  It "reports zero offset for empty file" {
    $dir = Join-Path $script:BaseTestDir ([Guid]::NewGuid().ToString("N").Substring(0, 8))
    New-Item -ItemType Directory -Path $dir -Force | Out-Null
    $filePath = Join-Path $dir "bridge-inbox.jsonl"
    New-Item -ItemType File -Path $filePath -Force | Out-Null

    $tracker = [ByteOffsetTracker]::new($filePath)
    $tracker.GetOffset() | Should -Be 0
  }

  It "detects appended content via offset change" {
    $dir = Join-Path $script:BaseTestDir ([Guid]::NewGuid().ToString("N").Substring(0, 8))
    New-Item -ItemType Directory -Path $dir -Force | Out-Null
    $filePath = Join-Path $dir "bridge-inbox.jsonl"
    New-Item -ItemType File -Path $filePath -Force | Out-Null

    $tracker = [ByteOffsetTracker]::new($filePath)
    $tracker.GetOffset() | Should -Be 0

    [System.IO.File]::WriteAllText($filePath, "test content")
    Start-Sleep -Milliseconds 100
    $tracker.Poll()
    $tracker.GetOffset() | Should -Be 12
  }

  It "does not detect unchanged file as new write" {
    $dir = Join-Path $script:BaseTestDir ([Guid]::NewGuid().ToString("N").Substring(0, 8))
    New-Item -ItemType Directory -Path $dir -Force | Out-Null
    $filePath = Join-Path $dir "bridge-inbox.jsonl"
    New-Item -ItemType File -Path $filePath -Force | Out-Null

    $tracker = [ByteOffsetTracker]::new($filePath)
    [System.IO.File]::WriteAllText($filePath, "static content")
    Start-Sleep -Milliseconds 100
    $tracker.Poll()
    $offsetAfterWrite = $tracker.GetOffset()

    Start-Sleep -Milliseconds 100
    $tracker.Poll()
    $tracker.GetOffset() | Should -Be $offsetAfterWrite
  }

  It "handles file truncation by resetting offset" {
    $dir = Join-Path $script:BaseTestDir ([Guid]::NewGuid().ToString("N").Substring(0, 8))
    New-Item -ItemType Directory -Path $dir -Force | Out-Null
    $filePath = Join-Path $dir "bridge-inbox.jsonl"
    [System.IO.File]::WriteAllText($filePath, "this is a long string of content")

    $tracker = [ByteOffsetTracker]::new($filePath)
    $tracker.GetOffset() | Should -Be 32

    [System.IO.File]::WriteAllText($filePath, "short")
    $result = $tracker.Poll()
    $result | Should -Be $true
    $tracker.GetOffset() | Should -Be 5
  }
}

Describe "DirectoryMutex" {
  BeforeEach {
    Get-ChildItem $script:BaseTestDir -Directory -ErrorAction SilentlyContinue | ForEach-Object {
      Remove-Item $_.FullName -Recurse -Force -ErrorAction SilentlyContinue
    }
  }

  It "acquires lock when directory does not exist" {
    $dir = Join-Path $script:BaseTestDir ([Guid]::NewGuid().ToString("N").Substring(0, 8))
    New-Item -ItemType Directory -Path $dir -Force | Out-Null
    $lockPath = Join-Path $dir "coordinator-lock"

    $mutex = [DirectoryMutex]::new($lockPath)
    $mutex.TryAcquire() | Should -Be $true
    Test-Path $lockPath | Should -Be $true
    $mutex.Release()
  }

  It "fails to acquire when lock already held" {
    $dir = Join-Path $script:BaseTestDir ([Guid]::NewGuid().ToString("N").Substring(0, 8))
    New-Item -ItemType Directory -Path $dir -Force | Out-Null
    $lockPath = Join-Path $dir "coordinator-lock"
    New-Item -ItemType Directory -Path $lockPath -Force | Out-Null

    $mutex = [DirectoryMutex]::new($lockPath)
    $mutex.TryAcquire() | Should -Be $false
  }

  It "releases lock and allows re-acquire" {
    $dir = Join-Path $script:BaseTestDir ([Guid]::NewGuid().ToString("N").Substring(0, 8))
    New-Item -ItemType Directory -Path $dir -Force | Out-Null
    $lockPath = Join-Path $dir "coordinator-lock"
    New-Item -ItemType Directory -Path $lockPath -Force | Out-Null

    $mutex1 = [DirectoryMutex]::new($lockPath)
    $mutex1.TryAcquire() | Should -Be $false

    Remove-Item $lockPath -Force
    $mutex2 = [DirectoryMutex]::new($lockPath)
    $mutex2.TryAcquire() | Should -Be $true
    $mutex2.Release()
  }

  It "release is idempotent when called twice" {
    $dir = Join-Path $script:BaseTestDir ([Guid]::NewGuid().ToString("N").Substring(0, 8))
    New-Item -ItemType Directory -Path $dir -Force | Out-Null
    $lockPath = Join-Path $dir "coordinator-lock"

    $mutex = [DirectoryMutex]::new($lockPath)
    $mutex.TryAcquire() | Should -Be $true
    $mutex.Release()
    { $mutex.Release() } | Should -Not -Throw
  }

  It "handles empty lock path gracefully" {
    $mutex = [DirectoryMutex]::new("")
    $mutex.TryAcquire() | Should -Be $false
  }
}
