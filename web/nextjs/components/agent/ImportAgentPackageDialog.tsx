"use client";

import { useEffect, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { X } from "lucide-react";
import { ModelPicker } from "@/components/agent/ModelPicker";
import {
  deleteAgent,
  importAgentPackage,
  previewAgentImport,
  type AgentPackBindings,
  type AgentPackDocument,
  type AgentPackPreview,
} from "@/lib/api/agents";
import { agentKeys } from "@/lib/hooks/useAgents";

interface ImportAgentPackageDialogProps {
  open: boolean;
  onClose: () => void;
  onImported?: (agentId: string) => void;
}

export function ImportAgentPackageDialog({
  open,
  onClose,
  onImported,
}: ImportAgentPackageDialogProps) {
  const queryClient = useQueryClient();
  const [parseError, setParseError] = useState("");
  const [previewing, setPreviewing] = useState(false);
  const [importing, setImporting] = useState(false);
  const [preview, setPreview] = useState<AgentPackPreview | null>(null);
  const [packDoc, setPackDoc] = useState<AgentPackDocument | null>(null);
  const [bindings, setBindings] = useState<AgentPackBindings>({});

  useEffect(() => {
    if (!open) {
      setParseError("");
      setPreview(null);
      setPackDoc(null);
      setBindings({});
      setPreviewing(false);
      setImporting(false);
    }
  }, [open]);

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape" && !importing) onClose();
    }
    if (open) document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [open, importing, onClose]);

  if (!open) return null;

  const issues = preview?.issues ?? [];
  const hasError = issues.some((i) => i.severity === "error");
  const needsModel = issues.some((i) => i.code === "model_unavailable");
  const hasGated = issues.some((i) => i.code === "gated_tool");

  async function onFile(file: File | undefined) {
    if (!file) return;
    setParseError("");
    setPreview(null);
    setPackDoc(null);
    let parsed: AgentPackDocument;
    try {
      const text = await file.text();
      parsed = JSON.parse(text) as AgentPackDocument;
    } catch {
      setParseError("That file is not valid JSON.");
      return;
    }
    if (parsed.kind !== "jobshout.agent") {
      setParseError("That file is not a JobShout agent package.");
      return;
    }
    setPreviewing(true);
    try {
      const report = await previewAgentImport(parsed);
      setPackDoc(parsed);
      setPreview(report);
      setBindings({ ...report.bindings });
    } catch (err) {
      setParseError(err instanceof Error ? err.message : "Validation failed");
    } finally {
      setPreviewing(false);
    }
  }

  async function onConfirm() {
    if (!preview || hasError) return;
    setImporting(true);
    try {
      const result = await importAgentPackage({
        preview_id: preview.preview_id,
        package: packDoc ?? undefined,
        bindings,
      });
      await queryClient.invalidateQueries({ queryKey: agentKeys.lists() });
      const name = result.agent.name;
      if (result.can_undo) {
        toast.success(`Imported “${name}”.`, {
          action: {
            label: "Undo",
            onClick: () => {
              void deleteAgent(result.agent.id).then(() => {
                void queryClient.invalidateQueries({ queryKey: agentKeys.lists() });
                toast.message("Import undone.");
              });
            },
          },
        });
      } else {
        toast.success(`Updated “${name}”.`);
      }
      onImported?.(result.agent.id);
      onClose();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Import failed");
    } finally {
      setImporting(false);
    }
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center p-4"
      role="dialog"
      aria-modal="true"
      aria-labelledby="import-agent-pack-title"
    >
      <div
        className="absolute inset-0 bg-black/60 backdrop-blur-sm"
        onClick={() => {
          if (!importing) onClose();
        }}
        aria-hidden="true"
      />
      <div className="relative z-10 flex max-h-[calc(100vh-6rem)] w-full max-w-lg flex-col rounded-lg border border-border bg-card shadow-xl">
        <div className="flex items-center justify-between border-b border-border px-6 py-4">
          <h2 id="import-agent-pack-title" className="text-base font-semibold">
            Import agent
          </h2>
          <button
            type="button"
            onClick={onClose}
            disabled={importing}
            className="rounded-sm text-muted-foreground hover:text-foreground disabled:opacity-50"
            aria-label="Close dialog"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        <div className="flex flex-col gap-4 overflow-y-auto px-6 py-5">
          <div>
            <input
              id="agent-pack-file"
              type="file"
              accept=".json,.jobshout-agent.json,application/json"
              className="sr-only"
              onChange={(e) => {
                const file = e.target.files?.[0];
                e.target.value = "";
                void onFile(file);
              }}
            />
            <label
              htmlFor="agent-pack-file"
              className="inline-flex h-9 cursor-pointer items-center rounded-md border border-border bg-background px-3 text-sm hover:bg-secondary"
            >
              Choose file
            </label>
            <p className="mt-1 text-xs text-muted-foreground">
              Select a .jobshout-agent.json export.
            </p>
          </div>

          {parseError && (
            <p className="rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive" role="alert">
              {parseError}
            </p>
          )}
          {previewing && (
            <p className="text-sm text-muted-foreground">Validating package…</p>
          )}

          {preview && (
            <>
              <div className="rounded-lg border border-border bg-background p-4">
                <p className="font-medium">{preview.agent.name}</p>
                <p className="text-sm text-muted-foreground">
                  {preview.agent.role}
                  {preview.agent.model_provider
                    ? ` · ${preview.agent.model_provider}${preview.agent.model_name ? ` / ${preview.agent.model_name}` : ""}`
                    : ""}
                </p>
                {preview.mode === "overlay" && (
                  <p className="mt-2 text-sm text-amber-700 dark:text-amber-400">
                    This organisation already has {preview.target_name ?? preview.agent.name}.
                    Import will update its prompt, model, tools, skills, and knowledge.
                    Credentials are not in the file — reconnect on the agent tab if needed.
                    This cannot be undone from this dialog.
                  </p>
                )}
                {preview.diff && (
                  <ul className="mt-2 list-inside list-disc text-sm text-muted-foreground">
                    {preview.diff.prompt_changed && <li>System prompt will change.</li>}
                    {preview.diff.model_changed && <li>Model preference will change.</li>}
                    {(preview.diff.tools_added?.length ?? 0) > 0 && (
                      <li>Tools added: {preview.diff.tools_added?.join(", ")}</li>
                    )}
                    {(preview.diff.tools_removed?.length ?? 0) > 0 && (
                      <li>Tools removed: {preview.diff.tools_removed?.join(", ")}</li>
                    )}
                    {preview.diff.knowledge_files > 0 && (
                      <li>
                        Knowledge files in the package ({preview.diff.knowledge_files}) will replace existing files.
                      </li>
                    )}
                    {preview.diff.skills > 0 && (
                      <li>Enabled skills in the package ({preview.diff.skills}) will replace the current set.</li>
                    )}
                  </ul>
                )}
                {preview.agent.description && (
                  <p className="mt-2 line-clamp-3 text-sm text-muted-foreground">
                    {preview.agent.description}
                  </p>
                )}
              </div>

              {preview.mode === "create" && (
                <div>
                  <label htmlFor="import-agent-name" className="block text-sm font-medium">
                    Name
                  </label>
                  <input
                    id="import-agent-name"
                    value={bindings.name ?? ""}
                    onChange={(e) =>
                      setBindings((b) => ({ ...b, name: e.target.value }))
                    }
                    className="mt-1.5 flex h-9 w-full rounded-md border border-input bg-background px-3 text-sm"
                  />
                </div>
              )}

              {needsModel && (
                <div>
                  <p className="mb-1 text-sm font-medium">Model</p>
                  <ModelPicker
                    value={{
                      provider: bindings.model_provider ?? "",
                      model: bindings.model_name ?? "",
                    }}
                    onChange={(v) =>
                      setBindings((b) => ({
                        ...b,
                        model_provider: v.provider,
                        model_name: v.model,
                      }))
                    }
                  />
                </div>
              )}

              {hasGated && (
                <label className="flex items-start gap-2 text-sm">
                  <input
                    type="checkbox"
                    className="mt-0.5"
                    checked={Boolean(bindings.include_gated_tools)}
                    onChange={(e) =>
                      setBindings((b) => ({
                        ...b,
                        include_gated_tools: e.target.checked,
                      }))
                    }
                  />
                  Include gated tools (for example shell_command)
                </label>
              )}

              {issues.length > 0 && (
                <ul className="space-y-1.5">
                  {issues.map((issue, i) => (
                    <li
                      key={`${issue.code}-${i}`}
                      className={`rounded-md border px-3 py-2 text-sm ${
                        issue.severity === "error"
                          ? "border-destructive/40 bg-destructive/10 text-destructive"
                          : issue.severity === "warning"
                            ? "border-amber-500/30 bg-amber-500/10 text-amber-800 dark:text-amber-200"
                            : "border-border bg-muted/50 text-muted-foreground"
                      }`}
                      role={issue.severity === "error" ? "alert" : undefined}
                    >
                      {issue.message}
                    </li>
                  ))}
                </ul>
              )}
            </>
          )}
        </div>

        <div className="flex justify-end gap-3 border-t border-border px-6 py-4">
          <button
            type="button"
            onClick={onClose}
            disabled={importing}
            className="inline-flex h-9 items-center rounded-md border border-border px-4 text-sm disabled:opacity-50"
          >
            Cancel
          </button>
          <button
            type="button"
            onClick={() => void onConfirm()}
            disabled={!preview || hasError || importing || previewing}
            className="inline-flex h-9 items-center rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground disabled:opacity-50"
          >
            {importing ? "Importing…" : preview?.mode === "overlay" ? "Update agent" : "Import"}
          </button>
        </div>
      </div>
    </div>
  );
}
