// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { WaveUIMessagePart } from "./aitypes";

export function getEffectiveApprovalStatus(baseApproval: string | undefined, isStreaming: boolean): string | undefined {
    return !isStreaming && baseApproval === "needs-approval" ? "timeout" : baseApproval;
}

export function getToolUseSignature(parts: Array<WaveUIMessagePart & { type: "data-tooluse" }>): string {
    return parts.map((part) => part.data.toolcallid).join("|");
}

export function getPendingApprovalSignature(
    parts: Array<WaveUIMessagePart & { type: "data-tooluse" }>,
    isStreaming: boolean
): string {
    return parts
        .filter((part) => getEffectiveApprovalStatus(part.data.approval, isStreaming) === "needs-approval")
        .map((part) => part.data.toolcallid)
        .join("|");
}
