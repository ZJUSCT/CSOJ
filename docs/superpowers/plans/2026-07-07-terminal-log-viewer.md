# WebGL Terminal Log Viewer — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the primitive div+span log viewers with an xterm.js + WebGL-accelerated terminal using JetBrains Mono font, ANSI color support, and auto-scroll.

**Architecture:** A new shared `TerminalLogViewer` component wraps xterm.js with the WebGL addon. It supports two modes: realtime (WebSocket) and static (NDJSON file). The existing `AdminSubmissionLogViewer` and `SubmissionLogViewer` components are updated to use it internally.

**Tech Stack:** Next.js 14, React 18, TypeScript, @xterm/xterm, @xterm/addon-webgl, @xterm/addon-fit.

**Reference spec:** `docs/superpowers/specs/2026-07-07-terminal-log-viewer-design.md`

**Branch:** `merge-webui-admin`.

---

## Task 1: Install deps + load JetBrains Mono font

**Files:**
- Modify: `frontend/package.json` (via pnpm)
- Modify: `frontend/app/layout.tsx`

- [ ] **Step 1: Install xterm packages**

Run:
```bash
cd frontend && pnpm add @xterm/xterm @xterm/addon-webgl @xterm/addon-fit
```

- [ ] **Step 2: Add JetBrains Mono via next/font/google**

In `frontend/app/layout.tsx`, add the font import alongside the existing `Inter` import:

```tsx
import { Inter, JetBrains_Mono } from "next/font/google";
```

Create the font instance:
```tsx
const jetbrainsMono = JetBrains_Mono({ subsets: ["latin"], variable: "--font-jetbrains-mono" });
```

Apply the CSS variable to the `<body>` className (add `jetbrainsMono.variable` alongside `inter.variable`):
```tsx
      <body
        className={cn(
          "min-h-screen bg-background font-sans antialiased",
          inter.variable,
          jetbrainsMono.variable
        )}
      >
```

- [ ] **Step 3: Verify**

Run: `cd frontend && pnpm build`
Expected: succeeds (font loaded, xterm installed but unused yet).

- [ ] **Step 4: Commit**

```bash
git add frontend/package.json frontend/pnpm-lock.yaml frontend/app/layout.tsx
git commit -m "build: add @xterm/xterm + WebGL addon + JetBrains Mono font"
```

---

## Task 2: Create `TerminalLogViewer` shared component

**Files:**
- Create: `frontend/components/shared/terminal-log-viewer.tsx`

- [ ] **Step 1: Create the component**

Create `frontend/components/shared/terminal-log-viewer.tsx`:

```tsx
"use client";

import { useRef, useEffect } from "react";
import { Terminal } from "@xterm/xterm";
import { WebglAddon } from "@xterm/addon-webgl";
import { FitAddon } from "@xterm/addon-fit";
import useWebSocket, { ReadyState } from "react-use-websocket";
import useSWR from "swr";
import api from "@/lib/api";
import "@xterm/xterm/css/xterm.css";

interface LogMessage {
  stream: string;
  data: string;
}

interface TerminalLogViewerProps {
  mode: "realtime" | "static";
  wsUrl?: string | null;
  staticLogUrl?: string;
  onStatusUpdate?: () => void;
}

function colorize(stream: string, data: string): string {
  switch (stream) {
    case "stderr":
      return `\x1b[31m${data}\x1b[0m`;
    case "info":
      return `\x1b[34m${data}\x1b[0m`;
    case "error":
      return `\x1b[31m\x1b[1m${data}\x1b[0m`;
    default:
      return data;
  }
}

function writeToTerminal(term: Terminal, msg: LogMessage) {
  term.write(colorize(msg.stream, msg.data));
}

export function TerminalLogViewer({ mode, wsUrl, staticLogUrl, onStatusUpdate }: TerminalLogViewerProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const termRef = useRef<Terminal | null>(null);
  const fitRef = useRef<FitAddon | null>(null);

  // Initialize terminal on mount
  useEffect(() => {
    if (!containerRef.current) return;

    const term = new Terminal({
      fontFamily: "'JetBrains Mono', monospace",
      fontSize: 13,
      scrollback: 10000,
      convertEol: false,
      disableStdin: true,
      cursorBlink: false,
      theme: {
        background: "#1e1e2e",
        foreground: "#cdd6f4",
      },
    });

    const fitAddon = new FitAddon();
    term.loadAddon(fitAddon);
    term.open(containerRef.current);

    // Try WebGL, fall back to canvas if it fails
    try {
      const webglAddon = new WebglAddon();
      term.loadAddon(webglAddon);
    } catch {
      // WebGL not available — xterm falls back to DOM renderer automatically
    }

    fitAddon.fit();
    termRef.current = term;
    fitRef.current = fitAddon;

    const handleResize = () => fitAddon.fit();
    window.addEventListener("resize", handleResize);

    return () => {
      window.removeEventListener("resize", handleResize);
      term.dispose();
      termRef.current = null;
    };
  }, []);

  // Static mode: load NDJSON from file
  const textFetcher = (url: string) => api.get(url, { responseType: "text" }).then(res => res.data);
  const { data: logText } = useSWR(
    mode === "static" && staticLogUrl ? staticLogUrl : null,
    textFetcher
  );

  useEffect(() => {
    if (mode !== "static" || !logText || !termRef.current) return;

    const lines = logText.split("\n").filter((line: string) => line.trim() !== "");
    for (const line of lines) {
      try {
        const msg = JSON.parse(line) as LogMessage;
        writeToTerminal(termRef.current, msg);
      } catch {
        termRef.current.write(line + "\r\n");
      }
    }
    termRef.current.scrollToBottom();
  }, [mode, logText]);

  // Realtime mode: WebSocket
  const { lastMessage, readyState } = useWebSocket(
    mode === "realtime" && wsUrl ? wsUrl : null,
    {
      shouldReconnect: () => false,
    }
  );

  useEffect(() => {
    if (mode !== "realtime" || !lastMessage || !termRef.current) return;
    try {
      const msg = JSON.parse(lastMessage.data) as LogMessage;
      writeToTerminal(termRef.current, msg);
      termRef.current.scrollToBottom();
    } catch {
      termRef.current.write(lastMessage.data);
    }
  }, [mode, lastMessage]);

  useEffect(() => {
    if (mode !== "realtime" || readyState === ReadyState.Closed || !termRef.current) return;
    if (readyState === ReadyState.Open) {
      termRef.current.write("\r\n\x1b[90m[Connected]\x1b[0m\r\n");
    }
  }, [mode, readyState]);

  // Status update polling
  useEffect(() => {
    if (mode !== "realtime" || !onStatusUpdate) return;
    const interval = setInterval(onStatusUpdate, 3000);
    return () => clearInterval(interval);
  }, [mode, onStatusUpdate, readyState]);

  return (
    <div
      ref={containerRef}
      className="h-[60vh] w-full overflow-hidden rounded-md"
      style={{ padding: "4px" }}
    />
  );
}
```

- [ ] **Step 2: Verify build**

Run: `cd frontend && pnpm build`
Expected: succeeds (component exists, not yet imported by pages).

- [ ] **Step 3: Commit**

```bash
git add frontend/components/shared/terminal-log-viewer.tsx
git commit -m "feat(frontend): TerminalLogViewer with xterm.js + WebGL + JetBrains Mono"
```

---

## Task 3: Integrate into AdminSubmissionLogViewer

**Files:**
- Modify: `frontend/components/admin/admin-submission-log-viewer.tsx`

- [ ] **Step 1: Replace the internal viewers with TerminalLogViewer**

Read the current `admin-submission-log-viewer.tsx`. It has:
- `StaticLogViewer` (NDJSON from `/admin/submissions/.../log`)
- `RealtimeLogViewer` (WebSocket)
- `AdminSubmissionLogViewer` (the exported shell with Tabs + WS URL construction)

Replace `StaticLogViewer` and `RealtimeLogViewer` with the shared component. The exported `AdminSubmissionLogViewer` shell stays — only the inner rendering changes:

In the `TabsContent` for a container that is `Running`:
```tsx
<TerminalLogViewer mode="realtime" wsUrl={getWsUrl(c.id)} onStatusUpdate={onStatusUpdate} />
```

In the `TabsContent` for a container that is `Success` or `Failed`:
```tsx
<TerminalLogViewer mode="static" staticLogUrl={`/admin/submissions/${submission.id}/containers/${c.id}/log`} />
```

Remove the `StaticLogViewer` and `RealtimeLogViewer` internal components entirely (they're replaced). Remove unused imports (`useSWR`, `Skeleton`, `useMemo`, `useEffect`, `useRef` — keep only what the shell still needs).

Add the import:
```tsx
import { TerminalLogViewer } from "@/components/shared/terminal-log-viewer";
```

- [ ] **Step 2: Verify build**

Run: `cd frontend && pnpm build`
Expected: succeeds.

- [ ] **Step 3: Commit**

```bash
git add frontend/components/admin/admin-submission-log-viewer.tsx
git commit -m "refactor(frontend): admin log viewer uses TerminalLogViewer"
```

---

## Task 4: Integrate into user SubmissionLogViewer

**Files:**
- Modify: `frontend/components/submissions/submission-log-viewer.tsx`

- [ ] **Step 1: Replace the internal viewers with TerminalLogViewer**

Same pattern as Task 3. Read the current `submission-log-viewer.tsx`. It has `StaticLogViewer` + `RealtimeLogViewer` + the exported `SubmissionLogViewer` shell.

Replace the inner viewers with `TerminalLogViewer`. The static log URL for users uses the user-facing path:
```tsx
<TerminalLogViewer mode="static" staticLogUrl={`/submissions/${submission.id}/containers/${c.id}/log`} />
```

The realtime WS URL is constructed the same way (with `?token=` query param). The `Show` flag check (whether the user can see this step's logs) stays in the shell.

Remove the `StaticLogViewer` and `RealtimeLogViewer` internal components. Remove unused imports.

Add the import:
```tsx
import { TerminalLogViewer } from "@/components/shared/terminal-log-viewer";
```

- [ ] **Step 2: Verify build**

Run: `cd frontend && pnpm build`
Expected: succeeds, 19 pages.

- [ ] **Step 3: Commit**

```bash
git add frontend/components/submissions/submission-log-viewer.tsx
git commit -m "refactor(frontend): user log viewer uses TerminalLogViewer"
```

---

## Task 5: Build + verify

**Files:** (no changes — verification)

- [ ] **Step 1: Full build**

Run: `cd frontend && pnpm build`
Expected: 19 pages, succeeds.

Run: `cd .. && go build ./... && go vet ./...`
Expected: clean.

Run: `make build`
Expected: binary produced.

- [ ] **Step 2: Manual smoke test**

Start the server. Open a submission detail page. The log viewer should render as an xterm terminal with:
- JetBrains Mono font
- Dark background (#1e1e2e)
- ANSI-colored output (stderr=red, info=blue, stdout=default)
- WebGL-accelerated rendering
- Auto-scroll to bottom on new output

- [ ] **Step 3: No commit**

---

## Self-Review notes

- `@xterm/xterm/css/xterm.css` must be imported in the component (not in `_app.tsx` or `layout.tsx`) — Next.js App Router supports CSS imports in client components.
- The terminal's `theme` uses a dark background (#1e1e2e) regardless of the app's light/dark mode — this is intentional for a terminal (terminals are conventionally dark). If the app is in light mode, the terminal still appears dark (like VS Code's integrated terminal).
- `disableStdin: true` makes the terminal read-only (no user input).
- The WebGL addon is wrapped in try/catch — if WebGL is unavailable (e.g. headless browser), xterm falls back to the DOM renderer.
- `FitAddon` handles resize on window resize; the container div has a fixed height (`h-[60vh]`).
- The `scrollToBottom()` call after each write ensures the latest output is visible.
- The `onStatusUpdate` polling interval (3s) matches the original viewer's behavior.
- The user-facing `submission-log-viewer.tsx` has a `Show` flag check on workflow steps — this logic stays in the shell, not in `TerminalLogViewer`.
