// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0
//
// wavehydrate emits the HYDRATE_WAVE_ASSISTANT v1 first-message contract
// from the on-disk snapshot file. The output is the canonical "first
// message" to any fresh Wave AI instance that should serve the
// `wave_assistant` lane. See S:\waveterm\HYDRATE_WAVE_ASSISTANT_v1.md for
// the spec.
//
// Usage:
//   wavehydrate                                  # default snapshot path
//   wavehydrate --snapshot <path>                # custom snapshot
//   wavehydrate --check                          # exit non-zero if snapshot stale (TTL)
//   wavehydrate --check && wavehydrate > msg.txt # CI gate + emit
//
// Exit codes:
//   0 success
//   1 invocation / IO error
//   2 snapshot structurally invalid
//   3 snapshot stale (TTL elapsed) and --check was requested
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const defaultSnapshotRel = `wave_assistant_context_v1.json`

// resolveSnapshotPath returns absolute path to the snapshot JSON file.
// Lookup order:
//   1. --snapshot flag
//   2. WAVETERM_SNAPSHOT env
//   3. ./wave_assistant_context_v1.json  (cwd)
//   4. S:\waveterm\wave_assistant_context_v1.json  (well-known user root)
//   5. <exe-dir>/wave_assistant_context_v1.json  (next to binary)
func resolveSnapshotPath(flagValue string) (string, error) {
	candidates := []string{}
	if flagValue != "" {
		if abs, err := filepath.Abs(flagValue); err == nil {
			candidates = append(candidates, abs)
		} else {
			candidates = append(candidates, flagValue)
		}
	}
	if env := os.Getenv("WAVETERM_SNAPSHOT"); env != "" {
		candidates = append(candidates, env)
	}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(cwd, defaultSnapshotRel))
	}
	candidates = append(candidates, `S:\waveterm\wave_assistant_context_v1.json`)
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), defaultSnapshotRel))
	}
	for _, p := range candidates {
		if p == "" {
			continue
		}
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("snapshot not found in any candidate path: %s", strings.Join(candidates, "; "))
}

// optionalHeader is the wire prefix the spec mandates. Bumping version is the
// only point of edit — bumps MUST match the spec doc and the JSON's
// `version` (if present).
const wireHeader = "HYDRATE_WAVE_ASSISTANT v1"

func main() {
	checkMode := flag.Bool("check", false, "verify the snapshot is fresh (TTL); do not emit")
	snapshotFlag := flag.String("snapshot", "", `override snapshot path (default: lookup)`)
	flag.Parse()

	path, err := resolveSnapshotPath(*snapshotFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "wavehydrate: %v\n", err)
		os.Exit(1)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "wavehydrate: read %s: %v\n", path, err)
		os.Exit(1)
	}

	// Validate JSON parses & required top-level keys exist.
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		fmt.Fprintf(os.Stderr, "wavehydrate: invalid JSON in %s: %v\n", path, err)
		os.Exit(2)
	}
	for _, k := range []string{"lane_id", "role", "forbidden_claims", "evidence_refs", "conflict_policy"} {
		if _, ok := doc[k]; !ok {
			fmt.Fprintf(os.Stderr, "wavehydrate: snapshot missing required field %q\n", k)
			os.Exit(2)
		}
	}

	// Freshness check (TTL) — only when --check.
	if *checkMode {
		st, err := evaluateTTL(doc, raw)
		if err != nil {
			fmt.Fprintf(os.Stderr, "wavehydrate: TTL evaluation: %v\n", err)
			os.Exit(2)
		}
		if st.stale {
			fmt.Fprintf(os.Stderr, "wavehydrate: snapshot stale by %s (TTL=%ds, now=%s, snapshot=%s)\n",
				st.reason, st.ttlSeconds, st.now.Format(time.RFC3339), st.snapshotAt.Format(time.RFC3339))
			os.Exit(3)
		}
		// --check is exclusive of emission.
		return
	}

	// Emit: "HYDRATE_WAVE_ASSISTANT v1\n<raw JSON>\n"
	// We preserve the JSON's own formatting by passing raw through.
	out := string(raw)
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	// Print header, then a blank line for readability, then raw payload.
	// The spec says `<JSON payload on following lines>`, so we do that.
	fmt.Printf("%s\n\n%s", wireHeader, out)
}

// pickTimestamp returns the snapshot_created_at string wherever the spec
// places it. Tries (1) verified_facts.snapshot_created_at, (2) top-level.
// Returns "" if neither exists.
func pickTimestamp(doc map[string]any) string {
	if vf, ok := doc["verified_facts"].(map[string]any); ok {
		if v, ok := vf["snapshot_created_at"].(string); ok {
			return v
		}
	}
	if v, ok := doc["snapshot_created_at"].(string); ok {
		return v
	}
	return ""
}

type ttlState struct {
	stale      bool
	reason     string
	ttlSeconds int
	now         time.Time
	snapshotAt time.Time
}

// evaluateTTL parses the snapshot's snapshot_created_at and ttl_sec and
// reports whether the snapshot is past its freshness window.
//
// Note: by spec, snapshot_created_at is INSIDE verified_facts (as one of the
// verified substrings of the payload, alongside timestamps of each evidence
// item). For forward-compatibility we also accept it at top-level.
func evaluateTTL(doc map[string]any, raw []byte) (*ttlState, error) {
	rawTTL, ok := doc["ttl_sec"].(float64)
	if !ok {
		return nil, fmt.Errorf("ttl_sec missing or not a number")
	}
	ts := pickTimestamp(doc)
	if ts == "" {
		return nil, fmt.Errorf("snapshot_created_at missing at verified_facts.* and top-level")
	}
	// First try RFC3339Nano, fall back to RFC3339.
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		t, err = time.Parse(time.RFC3339, ts)
		if err != nil {
			return nil, fmt.Errorf("snapshot_created_at cannot be parsed: %w", err)
		}
	}
	now := time.Now().UTC()
	age := now.Sub(t.UTC())
	ttl := time.Duration(int64(rawTTL)) * time.Second
	result := &ttlState{
		ttlSeconds: int(rawTTL),
		now:        now,
		snapshotAt: t.UTC(),
	}
	if age > ttl {
		result.stale = true
		result.reason = fmt.Sprintf("age %s > ttl %s", age.Round(time.Second), ttl)
	}
	return result, nil
}
