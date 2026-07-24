/**
 * Copyright (c) 2017 The xterm.js authors. All rights reserved.
 * @license MIT
 */

// This file is a copy of the original xterm.js file, with the following changes:
// - removed the allowance for the scrollbar

import type { FitAddon as IFitApi } from "@xterm/addon-fit";
import type { ITerminalAddon, Terminal } from "@xterm/xterm";

interface IDimensions {
    width: number;
    height: number;
}

interface IRenderDimensions {
    css: {
        canvas: IDimensions;
        cell: IDimensions;
    };
    device: {
        canvas: IDimensions;
        cell: IDimensions;
    };
}

interface ITerminalDimensions {
    /**
     * The number of rows in the terminal.
     */
    rows: number;

    /**
     * The number of columns in the terminal.
     */
    cols: number;
}

const MINIMUM_COLS = 2;
const MINIMUM_ROWS = 1;

export class FitAddon implements ITerminalAddon, IFitApi {
    private _terminal: Terminal | undefined;
    public scrollbarWidth: number | null = null;

    public activate(terminal: Terminal): void {
        this._terminal = terminal;
    }

    public dispose(): void {}

    public fit(): void {
        const dims = this.proposeDimensions();
        if (!dims || !this._terminal || isNaN(dims.cols) || isNaN(dims.rows)) {
            return;
        }

        // _core._renderService.clear() has no public equivalent; the resize() call
        // below triggers a full re-render internally, so skipping the explicit clear
        // is safe.  If visual artifacts appear on resize, re-evaluate xterm.js version
        // for a public API addition.
        if (this._terminal.rows !== dims.rows || this._terminal.cols !== dims.cols) {
            this._terminal.resize(dims.cols, dims.rows);
        }
    }

    public proposeDimensions(): ITerminalDimensions | undefined {
        if (!this._terminal) {
            return undefined;
        }

        if (!this._terminal.element || !this._terminal.element.parentElement) {
            return undefined;
        }

        // _core._renderService.dimensions is the only way to get per-cell pixel
        // dimensions in xterm.js; no public API exists for this.
        const core = (this._terminal as any)._core;
        const dims: IRenderDimensions = core._renderService.dimensions;

        if (dims.css.cell.width === 0 || dims.css.cell.height === 0) {
            return undefined;
        }

        let scrollbarWidth: number;
        if (this.scrollbarWidth != null) {
            scrollbarWidth = this.scrollbarWidth;
        } else {
            // Compute scrollbar width from public DOM: viewport element includes the
            // scroll area; the difference between its content width and the scroll
            // area offset gives the scrollbar width.
            const viewport = this._terminal.element.querySelector('.xterm-viewport') as HTMLElement | null;
            if (viewport) {
                scrollbarWidth = viewport.clientWidth - viewport.scrollWidth;
                if (scrollbarWidth < 0) {
                    scrollbarWidth = 0;
                }
            } else {
                scrollbarWidth = core.viewport?._viewportElement?.offsetWidth - core.viewport?._scrollArea?.offsetWidth || 0;
            }
        }

        const parentElementStyle = window.getComputedStyle(this._terminal.element.parentElement);
        const parentElementHeight = parseInt(parentElementStyle.getPropertyValue("height"));
        const parentElementWidth = Math.max(0, parseInt(parentElementStyle.getPropertyValue("width")));
        const elementStyle = window.getComputedStyle(this._terminal.element);
        const elementPadding = {
            top: parseInt(elementStyle.getPropertyValue("padding-top")),
            bottom: parseInt(elementStyle.getPropertyValue("padding-bottom")),
            right: parseInt(elementStyle.getPropertyValue("padding-right")),
            left: parseInt(elementStyle.getPropertyValue("padding-left")),
        };
        const elementPaddingVer = elementPadding.top + elementPadding.bottom;
        const elementPaddingHor = elementPadding.right + elementPadding.left;
        const availableHeight = parentElementHeight - elementPaddingVer;
        const availableWidth = parentElementWidth - elementPaddingHor - scrollbarWidth;
        const geometry = {
            cols: Math.max(MINIMUM_COLS, Math.floor(availableWidth / dims.css.cell.width)),
            rows: Math.max(MINIMUM_ROWS, Math.floor(availableHeight / dims.css.cell.height)),
        };
        return geometry;
    }
}
