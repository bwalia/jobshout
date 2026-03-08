"use client";

interface KnowledgeFile {
  id: string;
  name: string;
  updated_at: string;
}

interface KnowledgeFileListProps {
  files: KnowledgeFile[];
  selectedFileId: string;
  onSelectFile: (fileId: string) => void;
}

function formatRelativeTime(isoTimestamp: string): string {
  const diffMs = Date.now() - new Date(isoTimestamp).getTime();
  const diffSeconds = Math.floor(diffMs / 1000);

  if (diffSeconds < 60) return "just now";
  if (diffSeconds < 3600) return `${Math.floor(diffSeconds / 60)}m ago`;
  if (diffSeconds < 86_400) return `${Math.floor(diffSeconds / 3600)}h ago`;
  return `${Math.floor(diffSeconds / 86_400)}d ago`;
}

export function KnowledgeFileList({
  files,
  selectedFileId,
  onSelectFile,
}: KnowledgeFileListProps) {
  return (
    <div className="flex flex-col py-2">
      <p className="px-4 pb-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
        Files ({files.length})
      </p>

      <ul className="space-y-0.5">
        {files.map((file) => {
          const isActive = file.id === selectedFileId;
          return (
            <li key={file.id}>
              <button
                type="button"
                onClick={() => onSelectFile(file.id)}
                className={[
                  "flex w-full flex-col items-start rounded-none px-4 py-2 text-left transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring",
                  isActive
                    ? "bg-primary/10 text-primary"
                    : "text-foreground hover:bg-accent hover:text-accent-foreground",
                ].join(" ")}
              >
                {/* File icon + name */}
                <span className="flex items-center gap-2 text-sm font-medium leading-tight">
                  <svg
                    className="h-4 w-4 shrink-0 opacity-70"
                    xmlns="http://www.w3.org/2000/svg"
                    fill="none"
                    viewBox="0 0 24 24"
                    stroke="currentColor"
                    strokeWidth={2}
                  >
                    <path
                      strokeLinecap="round"
                      strokeLinejoin="round"
                      d="M9 12h6m-6 4h6m2 5H7a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5.586a1 1 0 0 1 .707.293l5.414 5.414a1 1 0 0 1 .293.707V19a2 2 0 0 1-2 2z"
                    />
                  </svg>
                  <span className="truncate">{file.name}</span>
                </span>

                {/* Last updated timestamp */}
                <span
                  className={[
                    "mt-0.5 pl-6 text-xs",
                    isActive ? "text-primary/70" : "text-muted-foreground",
                  ].join(" ")}
                >
                  {formatRelativeTime(file.updated_at)}
                </span>
              </button>
            </li>
          );
        })}
      </ul>

      {files.length === 0 && (
        <p className="px-4 py-6 text-center text-sm text-muted-foreground">
          No knowledge files yet. Click &ldquo;New File&rdquo; to create one.
        </p>
      )}
    </div>
  );
}
