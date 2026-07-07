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
    if (mode !== "realtime" || readyState === ReadyState.CLOSED || !termRef.current) return;
    if (readyState === ReadyState.OPEN) {
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
