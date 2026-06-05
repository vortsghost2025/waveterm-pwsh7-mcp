// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from "vitest";
import { WaveUIMessagePart } from "./aitypes";
import { getEffectiveApprovalStatus, getPendingApprovalSignature, getToolUseSignature } from "./aitooluse-utils";

function makeToolUsePart(
    toolcallid: string,
    approval: "needs-approval" | "user-approved" | "user-denied" | "auto-approved" | "timeout" = "needs-approval"
): WaveUIMessagePart & { type: "data-tooluse" } {
    return {
        type: "data-tooluse",
        data: {
            toolcallid,
            toolname: "grep",
            tooldesc: `tool ${toolcallid}`,
            status: "pending",
            approval,
        },
    } as WaveUIMessagePart & { type: "data-tooluse" };
}

describe("aitooluse utils", () => {
    it("treats non-streaming pending approvals as timed out", () => {
        expect(getEffectiveApprovalStatus("needs-approval", false)).toBe("timeout");
        expect(getEffectiveApprovalStatus("needs-approval", true)).toBe("needs-approval");
    });

    it("only includes currently pending tool calls in the approval signature", () => {
        const signature = getPendingApprovalSignature(
            [makeToolUsePart("grep-1"), makeToolUsePart("glob-2", "user-approved")],
            true
        );

        expect(signature).toBe("grep-1");
    });

    it("changes the pending approval signature when a new tool call arrives", () => {
        const firstSignature = getPendingApprovalSignature([makeToolUsePart("grep-1")], true);
        const secondSignature = getPendingApprovalSignature([makeToolUsePart("glob-2")], true);

        expect(secondSignature).not.toBe(firstSignature);
    });

    it("uses tool call ids to distinguish batched file operations", () => {
        const signature = getToolUseSignature([makeToolUsePart("read-1"), makeToolUsePart("read-2")]);

        expect(signature).toBe("read-1|read-2");
    });
});
