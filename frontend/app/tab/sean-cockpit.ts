// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { atoms, getApi, globalStore } from "@/app/store/global";

export const SeanCockpitPresetButtonLabel = "Sean Cockpit";

type PlaneBlockDef = { meta: { view: "term"; [key: string]: any } };

const cockpitBlockDefs: PlaneBlockDef[] = [
    { meta: { view: "term", "cmd:cwd": "S:\\waveterm" } },
    { meta: { view: "term", "cmd:cwd": "C:\\Users\\seand" } },
    { meta: { view: "term", connection: "ssh:root@2.25.206.123" } },
];

const cockpitSplitOrder: ("horizontal-after" | "vertical-before")[] = ["horizontal-after", "vertical-before"];

async function importCreateBlock() {
    const mod = await import("@/app/store/global");
    return mod.createBlock;
}

async function importCreateBlockSplitHorizontally() {
    const mod = await import("@/app/store/global");
    return mod.createBlockSplitHorizontally;
}

async function importCreateBlockSplitVertically() {
    const mod = await import("@/app/store/global");
    return mod.createBlockSplitVertically;
}

function waitForStaticTabId(prevTabId: string, timeoutMs: number): Promise<string> {
    return new Promise((resolve, reject) => {
        const start = Date.now();
        const check = () => {
            const cur = globalStore.get(atoms.staticTabId);
            if (cur && cur !== prevTabId) {
                resolve(cur);
                return;
            }
            if (Date.now() - start > timeoutMs) {
                reject(new Error("timed out waiting for new tab"));
                return;
            }
            setTimeout(check, 50);
        };
        check();
    });
}

export async function runSeanCockpitPreset(): Promise<void> {
    const api = getApi();
    if (api == null || typeof api.createTab !== "function") {
        console.warn("Sean Cockpit: electron createTab API not available");
        return;
    }

    const prevTabId = globalStore.get(atoms.staticTabId);
    api.createTab();

    try {
        await waitForStaticTabId(prevTabId ?? "", 5000);
    } catch (e) {
        console.warn("Sean Cockpit: createTab did not switch active tab", e);
        return;
    }

    const createBlock = await importCreateBlock();
    const createBlockSplitHorizontally = await importCreateBlockSplitHorizontally();
    const createBlockSplitVertically = await importCreateBlockSplitVertically();

    let firstBlockId: string;
    try {
        firstBlockId = await createBlock(cockpitBlockDefs[0] as any);
    } catch (e) {
        console.error("Sean Cockpit: failed to create root block", e);
        return;
    }

    try {
        await createBlockSplitHorizontally(cockpitBlockDefs[1] as any, firstBlockId, "after");
    } catch (e) {
        console.error("Sean Cockpit: failed to create second block", e);
        return;
    }

    try {
        await createBlockSplitVertically(cockpitBlockDefs[2] as any, firstBlockId, "before");
    } catch (e) {
        console.warn("Sean Cockpit: optional ssh block skipped", e);
    }

    console.log("Sean Cockpit: preset applied", { splitOrder: cockpitSplitOrder });
}
