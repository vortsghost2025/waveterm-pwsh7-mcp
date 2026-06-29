# Task: Schema-validation harness for all Strict-mode tools

Owner: opencode
Reviewer: wave-ai
Status: in-progress
Created: 2026-06-29 16:30 EDT

## Goal

Lock in OpenAI Responses API strict-mode compliance for every `Strict: true`
tool in `pkg/aiusechat/`. They must keep passing after future edits, not just
this once.

## Why

The `note_put` regression earlier today had to be caught by reading the
running log. That's a slow loop. We want a fast loop — every schema-touching
edit must be caught at `go test` time.

## Acceptance

1. Adding `Strict: true` to a tool without also adding all property names to
   `required` (including nested objects inside `items`, `properties`, etc.)
   fails `go test ./pkg/aiusechat/...`.
2. Test runs as part of the existing test binary — no new harness required to
   run it.
3. OpenCode can extend by editing a single, well-known place.

## Approach (draft)

A separate file `pkg/aiusechat/tools_strictschema_test.go` that:

- iterates a registered list of `GetXToolDefinition()` factories,
- for each, walks the schema tree and asserts:
  - every nested `type: object` has `required: <[]string of all prop keys>`,
  - every nested object has `additionalProperties: false`,
  - no schema-level `default` keys present at any level (defaults belong in the
    parse function with a `Description:` hint),
  - top-level `type: object` is present and a non-`any`-typed properties dict
    follows.
- registers every `GetXToolDefinition` in the `pkg/aiusechat` package via a
  helper slice.

A separate file exists already: `tools_validate_test.go` (141 lines). Need to
confirm it covers (1)–(4) above; extend if not.

## Deliverables

- `tools_strictschema.go` (or extension to `tools_validate_test.go`) with the
  validator + factory list.
- All `pkg/aiusechat` tests pass.
- A test failure demonstrates the harness: revert one required and show the
  test catches it. Then re-fix.

## Review

- [ ] wave-ai: confirm the validator's checks match `ConvertToolDefinitionToOpenAI`
      in `pkg/aiusechat/openai/openai-convertmessage.go` so we're testing the
      thing the API actually rejects.
- [ ] wave-ai: run from the assistant panel and report.

## Status

Status: draft → in-progress.
