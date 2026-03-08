"use client";

import { useState, useCallback } from "react";
import { KnowledgeFileList } from "@/components/agent/KnowledgeFileList";
import { KnowledgeEditor } from "@/components/agent/KnowledgeEditor";

interface KnowledgeFile {
  id: string;
  name: string;
  content: string;
  updated_at: string;
}

const INITIAL_FILES: KnowledgeFile[] = [
  {
    id: "1",
    name: "overview.md",
    content:
      "# Agent Overview\n\nThis document describes the agent's primary responsibilities and capabilities.\n",
    updated_at: new Date(Date.now() - 3600_000).toISOString(),
  },
  {
    id: "2",
    name: "instructions.md",
    content:
      "# Agent Instructions\n\n## Core Behaviour\n\n- Always respond in the user's language.\n- Prioritise accuracy over speed.\n",
    updated_at: new Date(Date.now() - 7200_000).toISOString(),
  },
  {
    id: "3",
    name: "domain-context.md",
    content: "# Domain Context\n\nBackground knowledge specific to this project.\n",
    updated_at: new Date(Date.now() - 86_400_000).toISOString(),
  },
];

let nextFileId = INITIAL_FILES.length + 1;

export default function KnowledgePage() {
  const [files, setFiles] = useState<KnowledgeFile[]>(INITIAL_FILES);
  const [selectedFileId, setSelectedFileId] = useState<string>(
    INITIAL_FILES[0]?.id ?? ""
  );

  const selectedFile = files.find((f) => f.id === selectedFileId) ?? null;

  // Update file content without marking it saved yet (dirty state managed inside editor)
  const handleContentChange = useCallback(
    (newContent: string) => {
      setFiles((prev) =>
        prev.map((f) =>
          f.id === selectedFileId ? { ...f, content: newContent } : f
        )
      );
    },
    [selectedFileId]
  );

  // Persist the current file (timestamps updated; in production this would hit the API)
  const handleSave = useCallback(() => {
    setFiles((prev) =>
      prev.map((f) =>
        f.id === selectedFileId
          ? { ...f, updated_at: new Date().toISOString() }
          : f
      )
    );
  }, [selectedFileId]);

  function handleNewFile(): void {
    const fileName = `document-${nextFileId}.md`;
    const newFile: KnowledgeFile = {
      id: String(nextFileId),
      name: fileName,
      content: `# ${fileName}\n\nStart writing here...\n`,
      updated_at: new Date().toISOString(),
    };
    nextFileId += 1;
    setFiles((prev) => [...prev, newFile]);
    setSelectedFileId(newFile.id);
  }

  function handleDeleteFile(): void {
    if (!selectedFile) return;

    // Guard: do not allow deleting the last file
    if (files.length === 1) return;

    const deletedIndex = files.findIndex((f) => f.id === selectedFileId);
    const remainingFiles = files.filter((f) => f.id !== selectedFileId);

    setFiles(remainingFiles);

    // Select the file before the deleted one, or the first file if deleted was first
    const nextIndex = Math.max(0, deletedIndex - 1);
    setSelectedFileId(remainingFiles[nextIndex]?.id ?? "");
  }

  return (
    <div className="flex h-[calc(100vh-4rem)] flex-col">
      {/* Page header */}
      <div className="flex items-center justify-between border-b border-border px-6 py-4">
        <div>
          <h1 className="text-xl font-semibold">Knowledge Base</h1>
          <p className="mt-0.5 text-sm text-muted-foreground">
            Manage the documents and context files available to this agent.
          </p>
        </div>

        <button
          type="button"
          onClick={handleNewFile}
          className="inline-flex h-9 items-center gap-2 rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground hover:bg-primary/90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <svg
            className="h-4 w-4"
            xmlns="http://www.w3.org/2000/svg"
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
            strokeWidth={2}
          >
            <path strokeLinecap="round" strokeLinejoin="round" d="M12 4v16m8-8H4" />
          </svg>
          New File
        </button>
      </div>

      {/* Split panel layout */}
      <div className="flex flex-1 overflow-hidden">
        {/* Left panel: file list */}
        <div className="w-64 shrink-0 overflow-y-auto border-r border-border">
          <KnowledgeFileList
            files={files}
            selectedFileId={selectedFileId}
            onSelectFile={setSelectedFileId}
          />
        </div>

        {/* Right panel: editor */}
        <div className="flex flex-1 flex-col overflow-hidden">
          {selectedFile ? (
            <>
              {/* Editor toolbar */}
              <div className="flex items-center justify-between border-b border-border px-4 py-2">
                <span className="text-sm font-medium text-muted-foreground">
                  {selectedFile.name}
                </span>
                <button
                  type="button"
                  onClick={handleDeleteFile}
                  disabled={files.length === 1}
                  className="inline-flex h-8 items-center gap-1.5 rounded-md border border-destructive/50 px-3 text-xs font-medium text-destructive transition-colors hover:bg-destructive/10 disabled:pointer-events-none disabled:opacity-40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                >
                  <svg
                    className="h-3.5 w-3.5"
                    xmlns="http://www.w3.org/2000/svg"
                    fill="none"
                    viewBox="0 0 24 24"
                    stroke="currentColor"
                    strokeWidth={2}
                  >
                    <path
                      strokeLinecap="round"
                      strokeLinejoin="round"
                      d="M19 7l-.867 12.142A2 2 0 0 1 16.138 21H7.862a2 2 0 0 1-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 0 0-1-1h-4a1 1 0 0 0-1 1v3M4 7h16"
                    />
                  </svg>
                  Delete
                </button>
              </div>

              <div className="flex-1 overflow-hidden">
                <KnowledgeEditor
                  value={selectedFile.content}
                  onChange={handleContentChange}
                  onSave={handleSave}
                />
              </div>
            </>
          ) : (
            <div className="flex flex-1 items-center justify-center text-muted-foreground">
              Select a file to start editing.
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
