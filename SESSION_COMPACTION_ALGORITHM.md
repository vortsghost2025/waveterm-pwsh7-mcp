# Kilo Session Compaction Algorithm Specification

## Overview
This document specifies the session compaction algorithm used in Kilo/OpenCode to manage context limits while preserving essential conversation state across sessions.

## Algorithm Components

### 1. Trigger Conditions
Compaction is triggered when:
- Token estimate exceeds model context limit (handled via `isOverflow` check)
- Explicit compaction request via `SessionCompaction.create()`
- Automatic compaction configured via settings

### 2. Pruning Phase (Context Reduction)
Before generating summary, old tool calls are pruned to free context space:

**Parameters:**
- `PRUNE_MINIMUM = 20_000` tokens (minimum to prune)
- `PRUNE_PROTECT = 40_000` tokens (protected threshold)
- `PRUNE_PROTECTED_TOOLS = ["skill"]` (tool types never pruned)

**Algorithm:**
1. Traverse messages backward from most recent
2. Skip first 2 user turns (preserve recent conversation)
3. Stop at summarized messages or compacted tool calls
4. For completed tool calls (excluding protected types):
   - Estimate token output using `Token.estimate()`
   - Accumulate total until exceeding `PRUNE_PROTECT`
   - Mark excess tool calls for pruning
5. For marked tool calls:
   - Set `part.state.time.compacted = Date.now()`
   - Persist via `session.updatePart()`

### 3. Summary Generation
After pruning, a compaction agent generates a structured summary:

**Agent Configuration:**
- Agent: "compaction" (primary mode, native, hidden)
- Prompt: Defined in `src/agent/prompt/compaction.txt`
- Model: Inherits from session/user configuration

**Prompt Instructions:**
```
You are a helpful AI assistant tasked with summarizing conversations.
When asked to summarize, provide a detailed but concise summary of the conversation.
Focus on information that would be helpful for continuing the conversation, including:
- What was done
- What is currently being worked on
- Which files are being modified
- What needs to be done next
- Key user requests, constraints, or preferences that should persist
- Important technical decisions and why they were made

Your summary should be comprehensive enough to provide context but concise enough to be quickly understood.
Do not respond to any questions in the conversation, only output the summary.
```

### 4. Continuity Message Creation
Post-summary, a synthetic user message is created to continue the session:

**Content:**
```
Continue if you have next steps, or stop and ask for clarification if you are unsure how to proceed.
```

**Properties:**
- Role: user
- Type: text
- Synthetic: true
- Attached to new user message via `session.updatePart()`

### 5. Event Emission
Upon successful compaction:
- Publishes `SessionCompaction.Event.Compacted` with `sessionID`
- Enables external systems to react to compaction completion

## Data Flow

### Inputs
- `parentID`: MessageID to compact from
- `messages`: Array of MessageV2.WithParts
- `sessionID`: Target session identifier
- `abort`: AbortSignal for cancellation
- `auto`: Boolean for automatic continuation
- `overflow`: Boolean indicating context overflow trigger

### Processing Steps
1. **Overflow Handling** (if `overflow=true`):
   - Find replay message (last user message before parent without compaction parts)
   - Truncate message set to preserve essential context
2. **Agent Selection**:
   - Retrieve "compaction" agent from Agent.Service
   - Obtain model from user message or agent configuration
3. **Plugin Processing**:
   - Allow plugins to inject context or replace prompt via `experimental.session.compacting`
4. **Message Preparation**:
   - Deep copy messages
   - Apply `experimental.chat.messages.transform` plugin
   - Convert to model messages via `MessageV2.toModelMessages()`
5. **Compaction Message Creation**:
   - Create assistant message with compaction role
   - Set mode="compaction", agent="compaction", summary=true
   - Include session context (cwd, root from Instance)
   - Initialize zero cost/token counters
6. **Processor Execution**:
   - Create SessionProcessor for the compaction message
   - Process with model messages + compaction prompt
   - Handle abort/cancellation gracefully
7. **Post-Processing**:
   - On success ("continue"): Create continuation message, replay if needed
   - On failure ("stop"): Mark message error, emit stop signal
   - On context overflow: Set ContextOverflowError, return "stop"

## Configuration
Compaction behavior controlled via `cfg.compaction`:
- `prune`: boolean (default true) - enable/disable pruning phase
- Additional settings via config system

## Integration Points
- **Session.Service**: Message/part persistence
- **Agent.Service**: Compaction agent retrieval
- **Plugin.Service**: Prompt/context injection
- **Bus.Service**: Event publishing
- **Config.Service**: Compaction settings