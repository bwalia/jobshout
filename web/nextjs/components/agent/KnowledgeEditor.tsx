"use client";

import { useState, useEffect, useRef, useCallback } from "react";
import dynamic from "next/dynamic";

// Monaco is a large browser-only library; load it dynamically to keep the
// server bundle small and avoid SSR issues with the window object.
const MonacoEditor = dynamic(() => import("@monaco-editor/react"), {
  ssr: false,
  loading: () => (
    <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
      Loading editor...
    </div>
  ),
});

type SaveState = "idle" | "saving" | "saved";

interface KnowledgeEditorProps {
  /** Current Markdown content of the file */
  value: string;
  /** Called on every content change */
  onChange: (value: string) => void;
  /** Called when the user explicitly requests a save (Ctrl/Cmd+S or Save button) */
  onSave: () => void;
}

// Delay in milliseconds before resetting the "Saved" indicator back to idle
const SAVED_RESET_DELAY_MS = 2000;

export function KnowledgeEditor({ value, onChange, onSave }: KnowledgeEditorProps) {
  const [saveState, setSaveState] = useState<SaveState>("idle");
  const savedResetTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Clean up the reset timer on unmount to avoid state updates on an unmounted component
  useEffect(() => {
    return () => {
      if (savedResetTimerRef.current) {
        clearTimeout(savedResetTimerRef.current);
      }
    };
  }, []);

  const triggerSave = useCallback(() => {
    if (savedResetTimerRef.current) {
      clearTimeout(savedResetTimerRef.current);
    }

    setSaveState("saving");

    // Simulate async save; in production onSave would return a Promise
    onSave();

    setSaveState("saved");

    savedResetTimerRef.current = setTimeout(() => {
      setSaveState("idle");
    }, SAVED_RESET_DELAY_MS);
  }, [onSave]);

  // Register Ctrl/Cmd+S keyboard shortcut on the editor mount
  function handleEditorDidMount(
    editor: { addCommand: (keybinding: number, handler: () => void) => void },
    monaco: {
      KeyMod: { CtrlCmd: number };
      KeyCode: { KeyS: number };
    }
  ): void {
    // eslint-disable-next-line no-bitwise
    editor.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.KeyS, triggerSave);
  }

  function handleChange(newValue: string | undefined): void {
    onChange(newValue ?? "");
    // Reset save indicator when the user makes a change after a save
    if (saveState === "saved") {
      setSaveState("idle");
    }
  }

  return (
    <div className="flex h-full flex-col">
      {/* Save status bar */}
      <div className="flex items-center justify-between border-b border-border bg-muted/30 px-4 py-1.5">
        <span className="text-xs text-muted-foreground">
          Markdown &bull; Use Ctrl/Cmd+S to save
        </span>

        <div className="flex items-center gap-3">
          {/* Save state indicator */}
          {saveState === "saving" && (
            <span className="text-xs text-muted-foreground animate-pulse">
              Saving...
            </span>
          )}
          {saveState === "saved" && (
            <span className="flex items-center gap-1 text-xs text-green-600">
              <svg
                className="h-3.5 w-3.5"
                xmlns="http://www.w3.org/2000/svg"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
                strokeWidth={2}
              >
                <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
              </svg>
              Saved
            </span>
          )}

          <button
            type="button"
            onClick={triggerSave}
            className="inline-flex h-7 items-center rounded-md bg-primary px-3 text-xs font-medium text-primary-foreground hover:bg-primary/90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            Save
          </button>
        </div>
      </div>

      {/* Monaco editor fills remaining height */}
      <div className="flex-1 overflow-hidden">
        <MonacoEditor
          height="100%"
          language="markdown"
          value={value}
          onChange={handleChange}
          onMount={handleEditorDidMount}
          options={{
            minimap: { enabled: false },
            wordWrap: "on",
            lineNumbers: "on",
            scrollBeyondLastLine: false,
            fontSize: 14,
            tabSize: 2,
            automaticLayout: true,
            padding: { top: 12, bottom: 12 },
          }}
          theme="vs-dark"
        />
      </div>
    </div>
  );
}
