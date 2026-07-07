# WebGL Terminal Log Viewer — Design

**Date:** 2026-07-07
**Scope:** Replace the current primitive `<div>`+`<span>` log viewers with an xterm.js + WebGL-accelerated terminal using JetBrains Mono font. A shared `TerminalLogViewer` component replaces the internal rendering of both the admin and user submission log viewers.

## Goals

1. **`@xterm/xterm.js` + `@xterm/addon-webgl`** — hardware-accelerated terminal rendering via WebGL canvas.
2. **JetBrains Mono font** — loaded via `next/font/google`, used as the terminal's `fontFamily`.
3. **Shared `TerminalLogViewer` component** — one component for both realtime (WS) and static (NDJSON file) log viewing. Replaces `RealtimeLogViewer` + `StaticLogViewer` in both `admin-submission-log-viewer.tsx` and `submission-log-viewer.tsx`.
4. **ANSI color support** — stdout (default), stderr (red), info (blue), error (red bold) via ANSI escape codes.
5. **Auto-scroll** — new output scrolls to the bottom automatically.

## Non-goals

- User input to the terminal (it's a read-only viewer).
- Persistent scrollback beyond xterm's default buffer (1000 lines; can be increased via `scrollback` option).
- Custom themes beyond light/dark matching the app's theme.
- Changing the backend WS message format or NDJSON log format.

---

## Component

### `frontend/components/shared/terminal-log-viewer.tsx` (new)

```tsx
interface TerminalLogViewerProps {
  mode: 'realtime' | 'static';
  wsUrl?: string;          // realtime: WebSocket URL
  staticLogUrl?: string;   // static: NDJSON log file URL
  onStatusUpdate?: () => void;
}
```

**Lifecycle:**
1. On mount: create `Terminal` instance, load `WebGLAddon` + `FitAddon`, open into a `<div ref>`, set `fontFamily: "'JetBrains Mono', monospace"`, `fontSize: 13`, `scrollback: 10000`, `convertEol: false`.
2. Fit the terminal to the container size (`fitAddon.fit()`); re-fit on window resize.
3. Mode-specific data ingestion (below).
4. On unmount: dispose the terminal + addons + close the WS.

**Realtime mode:**
- Connect WebSocket to `wsUrl`.
- On message: `JSON.parse(e.data)` → `{stream, data}` → write with ANSI color:
  - `stdout` → `terminal.write(data)` (no color)
  - `stderr` → `terminal.write('\x1b[31m' + data + '\x1b[0m')` (red)
  - `info` → `terminal.write('\x1b[34m' + data + '\x1b[0m')` (blue)
  - `error` → `terminal.write('\x1b[31m\x1b[1m' + data + '\x1b[0m')` (red bold)
- Auto-scroll: after each write, `terminal.scrollToBottom()`.
- On WS close: write `\r\n\x1b[90m[Connection closed]\x1b[0m\r\n`.
- `onStatusUpdate` called periodically (the existing poll for submission status).

**Static mode:**
- `api.get(staticLogUrl, {responseType:'text'})` → NDJSON string.
- Split by `\n`, filter empty, `JSON.parse` each line.
- Write each message with the same ANSI coloring as realtime.
- Scroll to bottom after all writes.

### ANSI color helper

```ts
function colorize(stream: string, data: string): string {
  switch (stream) {
    case 'stderr': return `\x1b[31m${data}\x1b[0m`;
    case 'info': return `\x1b[34m${data}\x1b[0m`;
    case 'error': return `\x1b[31m\x1b[1m${data}\x1b[0m`;
    default: return data; // stdout
  }
}
```

### CSS

xterm requires its own CSS (`@xterm/xterm/css/xterm.css`). Import it in the component. The container `<div>` needs a fixed height (e.g., `h-[60vh]`) and `position: relative` for the WebGL canvas to fill it.

---

## Font loading

In `frontend/app/layout.tsx`, add:
```tsx
import { JetBrains_Mono } from 'next/font/google';
const jetbrainsMono = JetBrains_Mono({ subsets: ['latin'], variable: '--font-jetbrains-mono' });
```
Apply `jetbrainsMono.variable` to the `<body>` className (alongside the existing `inter.variable`). The terminal component reads `getComputedStyle(document.documentElement).getPropertyValue('--font-jetbrains-mono')` or simply uses the string `"'JetBrains Mono', monospace"` directly in `terminal.options.fontFamily` — xterm's font loading is independent of next/font, so using the name string is sufficient (next/font makes the font available as a CSS variable that the browser resolves).

Actually, simpler: just set `fontFamily: "'JetBrains Mono', monospace"` in the terminal options. The `next/font/google` import in layout.tsx makes the font available on the page; the xterm canvas renderer reads the computed font-family of its container. No CSS variable needed — just ensure the font is loaded via `next/font/google` in layout.tsx and the name string matches.

---

## Integration points

### `frontend/components/admin/admin-submission-log-viewer.tsx`

The file currently has `RealtimeLogViewer` (WS) + `StaticLogViewer` (NDJSON). Replace both with `TerminalLogViewer`:
- Where `RealtimeLogViewer` was used → `<TerminalLogViewer mode="realtime" wsUrl={wsUrl} onStatusUpdate={onStatusUpdate} />`
- Where `StaticLogViewer` was used → `<TerminalLogViewer mode="static" staticLogUrl={`/admin/submissions/${submissionId}/containers/${containerId}/log`} />`

The outer shell (Tabs, WS URL construction, workflow step matching) stays unchanged.

### `frontend/components/submissions/submission-log-viewer.tsx`

Same pattern — replace the internal viewers with `<TerminalLogViewer>`. The static log URL uses the user-facing path: `/submissions/${submissionId}/containers/${containerId}/log`.

---

## Dependencies

```bash
cd frontend && pnpm add @xterm/xterm @xterm/addon-webgl @xterm/addon-fit
```

## Testing

- `pnpm build` — 19 pages, succeeds.
- Manual: submit a problem, watch the live log stream in the terminal. After completion, view the static log — both should render in the xterm terminal with JetBrains Mono + WebGL rendering.
- Verify: stderr is red, info markers are blue, stdout is default color. Auto-scroll works. Resize works (terminal re-fits).

## Out of scope

- User input to the terminal.
- Custom terminal themes (uses the default theme — white-on-black or adapted to app theme).
- Changing the backend WS/NDJSON format.
- Search within the terminal (xterm has a search addon but it's not in this scope).
