"use client";

import { useState, useEffect, useRef, useCallback } from "react";
import dynamic from "next/dynamic";
import { useTheme } from "next-themes";

const MonacoEditor = dynamic(() => import("@monaco-editor/react"), {
  ssr: false,
  loading: () => (
    <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
      Loading editor...
    </div>
  ),
});

type SaveState = "idle" | "saving" | "saved" | "error";

interface KnowledgeEditorProps {
  value: string;
  onChange: (value: string) => void;
  onSave: () => void | Promise<void>;
}

const SAVED_RESET_DELAY_MS = 2000;

export function KnowledgeEditor({ value, onChange, onSave }: KnowledgeEditorProps) {
  const [saveState, setSaveState] = useState<SaveState>("idle");
  const savedResetTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const { resolvedTheme } = useTheme();
  const [mounted, setMounted] = useState(false);

  useEffect(() => setMounted(true), []);

  useEffect(() => {
    return () => {
      if (savedResetTimerRef.current) {
        clearTimeout(savedResetTimerRef.current);
      }
    };
  }, []);

  const triggerSave = useCallback(async () => {
    if (savedResetTimerRef.current) {
      clearTimeout(savedResetTimerRef.current);
    }

    setSaveState("saving");
    try {
      await onSave();
      setSaveState("saved");
      savedResetTimerRef.current = setTimeout(() => {
        setSaveState("idle");
      }, SAVED_RESET_DELAY_MS);
    } catch {
      setSaveState("error");
    }
  }, [onSave]);

  function handleEditorDidMount(
    editor: { addCommand: (keybinding: number, handler: () => void) => void },
    monaco: {
      KeyMod: { CtrlCmd: number };
      KeyCode: { KeyS: number };
    }
  ): void {
    // eslint-disable-next-line no-bitwise
    editor.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.KeyS, () => {
      void triggerSave();
    });
  }

  function handleChange(newValue: string | undefined): void {
    onChange(newValue ?? "");
    if (saveState === "saved" || saveState === "error") {
      setSaveState("idle");
    }
  }

  const monacoTheme = mounted && resolvedTheme === "dark" ? "vs-dark" : "vs";

  return (
    <div className="flex h-full flex-col">
      <div className="flex items-center justify-between border-b border-border bg-muted/30 px-4 py-1.5">
        <span className="text-xs text-muted-foreground">
          Markdown &bull; Use Ctrl/Cmd+S to save
        </span>

        <div className="flex items-center gap-3">
          {saveState === "saving" && (
            <span className="text-xs text-muted-foreground animate-pulse">
              Saving...
            </span>
          )}
          {saveState === "saved" && (
            <span className="flex items-center gap-1 text-xs text-green-600 dark:text-green-400">
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
          {saveState === "error" && (
            <span className="text-xs text-destructive">Save failed</span>
          )}

          <button
            type="button"
            onClick={() => void triggerSave()}
            className="inline-flex h-7 items-center rounded-md bg-primary px-3 text-xs font-medium text-primary-foreground hover:bg-primary/90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            Save
          </button>
        </div>
      </div>

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
          theme={monacoTheme}
        />
      </div>
    </div>
  );
}
