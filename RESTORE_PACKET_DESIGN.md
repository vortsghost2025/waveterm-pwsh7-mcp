# Restore Packet Design for Kilo-Archivist Continuity

## Overview
This document defines the schema and protocol for restore packets used to maintain continuity between Kilo sessions and Archivist lanes during compaction and recovery operations.

## Restore Packet Schema (Based on COMPACT_RESTORE_PACKET.json v1.4)

### Top-Level Structure
```jsonc
{
  "schema_version": "1.4",  // Semantic version of packet format
  "timestamp": "ISO 8601 timestamp",
  "lane": "lane identifier (e.g., 'archivist')",
  "generator": "component that created packet",
  "generator_command": "command used to generate",
  "authority": {
    "fields_authoritative": ["string"],  // Fields requiring exact preservation
    "fields_advisory": ["string"]        // Fields with flexible interpretation
  },
  "restore_payload": {
    "governance_constraints": {
      "STRUCTURE_OVER_IDENTITY": boolean,
      "CORRECTION_MANDATORY": boolean,
      "SINGLE_ENTRY_POINT": boolean,
      "OPERATOR_ACCOUNTABILITY": boolean
    },
    "active_checkpoints": [],           // Array of checkpoint identifiers
    "drift_baseline": {
      "uds_score": number,              // User Drift Score
      "last_updated": "ISO timestamp | null"
    },
    "session_context": {
      "lane_id": "string",
      "role": "string",
      "governance_active": boolean
    },
    "working_context_resume": ["string"] // Hint strings for session resumption
  }
}
```

### Field Definitions

#### Authority Section
- **fields_authoritative**: Must be preserved exactly during restore (governance constraints)
- **fields_advisory**: Can be adapted based on current context (working context hints)

#### Governance Constraints
Boolean flags representing core architectural principles:
- `STRUCTURE_OVER_IDENTITY`: System structure takes precedence over agent identity
- `CORRECTION_MANDATORY`: Corrections must be applied, consensus optional
- `SINGLE_ENTRY_POINT`: All governance flows through BOOTSTRAP.md
- `OPERATOR_ACCOUNTABILITY`: Operators responsible for their actions

#### Session Context
Identifies the lane and role context for the session:
- `lane_id`: Identifier of the Archivist lane
- `role`: Functional role within the lane
- `governance_active`: Whether governance constraints are active

#### Working Context Resume
Array of hint strings to restore session state:
- Format: `"key:value"` or `"flag:true/false"`
- Examples: `"handoff_exists:true"`, `"blocker_active:false"`, `"trust_store_lanes:4"`

## Restore Protocol

### Packet Generation (During Compaction)
1. **Trigger**: SessionCompaction.process() completes compaction
2. **Source Data**: 
   - Lane registry (.global/lane-registry.json)
   - Current session state (SESSION_STATE_*.md)
   - Continuity registry (CONTINUITY_REGISTRY.json)
   - Governance documents (BOOTSTRAP.md, etc.)
3. **Process**:
   - Extract governance constraints from authoritative sources
   - Capture current checkpoint status
   - Measure drift baseline (UDS score)
   - Record session context (lane, role, governance status)
   - Generate working context hints from session state
   - Package into restore packet with schema_version and timestamp

### Packet Consumption (Session Initialization)
1. **Trigger**: New session startup or post-compaction restore
2. **Process**:
   - Load most recent restore packet for lane
   - Validate schema_version compatibility
   - Verify authority fields match current governance
   - Apply governance_constraints to session configuration
   - Restore session context from packet
   - Use working_context_resume to initialize session state
   - Validate continuity via fingerprint matching

### Validation Rules
1. **Schema Compliance**: Must conform to defined JSON structure
2. **Authority Validation**: fields_authoritative must match current governance
3. **Lane Consistency**: lane in packet must match target lane
4. **Temporal Validity**: timestamp must be within acceptable window
5. **Fingerprint Match**: Constitutional and continuity hashes must align

## Integration Points

### Kilo Session Compaction
- Modified SessionCompaction.Service to generate restore packet after compaction
- Packet stored in lane's .compact-audit/ directory
- Referenced in SESSION_STATE_*.md files

### Archivist Lane Initialization
- LaneDiscovery reads restore packet during startup
- Applies governance constraints to lane configuration
- Initializes session state from working context hints
- Validates continuity before activating lane

### Cross-Lane Communication
- Restore packets transferred via lane inbox/outbox system
- Processed by lane-watcher daemon
- Used for lane handoff and recovery scenarios

## Example Packet
```json
{
  "schema_version": "1.4",
  "timestamp": "2026-05-15T21:09:36.272Z",
  "lane": "archivist",
  "generator": "compact-restore-bridge.js",
  "generator_command": "generate-packet",
  "authority": {
    "fields_authoritative": [
      "governance_constraints",
      "active_checkpoints",
      "drift_baseline",
      "session_context"
    ],
    "fields_advisory": [
      "working_context_resume"
    ]
  },
  "restore_payload": {
    "governance_constraints": {
      "STRUCTURE_OVER_IDENTITY": true,
      "CORRECTION_MANDATORY": true,
      "SINGLE_ENTRY_POINT": true,
      "OPERATOR_ACCOUNTABILITY": true
    },
    "active_checkpoints": [],
    "drift_baseline": {
      "uds_score": 0,
      "last_updated": null
    },
    "session_context": {
      "lane_id": "archivist",
      "role": "archivist",
      "governance_active": true
    },
    "working_context_resume": [
      "handoff_exists:true",
      "blocker_active:false",
      "trust_store_lanes:4",
      "graph_snapshot_excluded:true"
    ]
  }
}
```

## Implementation Notes
1. **Backward Compatibility**: Schema version allows evolution
2. **Extensibility**: New fields can be added to authority sections
3. **Security**: Sensitive data should be referenced, not embedded
4. **Atomicity**: Packet generation should be atomic operation
5. **Validation**: Both ends should validate before acting on packet