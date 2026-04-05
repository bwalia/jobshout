"use client";

import { useState } from "react";
import {
  useSessions,
  useCreateSession,
  useUpdateSession,
  useDeleteSession,
  useCopySessionContext,
  useCreateSnapshot,
  useSessionSnapshots,
  useRestoreSnapshot,
} from "@/lib/hooks/useSessions";
import { useLLMProviders } from "@/lib/hooks/useLLMProviders";
import type { CreateSessionRequest, Session } from "@/lib/types/session";

export default function SessionsPage() {
  const { data: sessionsResponse, isLoading } = useSessions();
  const { data: providers } = useLLMProviders();
  const createMutation = useCreateSession();
  const updateMutation = useUpdateSession();
  const deleteMutation = useDeleteSession();
  const copyMutation = useCopySessionContext();
  const snapshotMutation = useCreateSnapshot();
  const restoreMutation = useRestoreSnapshot();

  const [showForm, setShowForm] = useState(false);
  const [selectedSession, setSelectedSession] = useState<string | null>(null);
  const [copySource, setCopySource] = useState("");
  const [showCopyDialog, setShowCopyDialog] = useState<string | null>(null);
  const [showSnapshotDialog, setShowSnapshotDialog] = useState<string | null>(null);
  const [snapshotName, setSnapshotName] = useState("");

  const [form, setForm] = useState<CreateSessionRequest>({
    name: "",
    description: "",
    tags: [],
  });

  const sessions = sessionsResponse?.data ?? [];

  async function handleCreate(e: React.FormEvent) {
    e.preventDefault();
    const s = await createMutation.mutateAsync(form);
    setShowForm(false);
    setSelectedSession(s.id);
    setForm({ name: "", description: "", tags: [] });
  }

  async function handleCopyContext(targetId: string) {
    if (!copySource) return;
    await copyMutation.mutateAsync({
      sessionId: targetId,
      payload: {
        source_session_id: copySource,
        include_system: false,
      },
    });
    setShowCopyDialog(null);
    setCopySource("");
  }

  async function handleSnapshot(sessionId: string) {
    if (!snapshotName) return;
    await snapshotMutation.mutateAsync({
      sessionId,
      payload: { name: snapshotName },
    });
    setShowSnapshotDialog(null);
    setSnapshotName("");
  }

  return (
    <div className="mx-auto max-w-5xl space-y-8">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">Session Manager</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Manage conversation sessions, copy context across LLMs, and save snapshots.
          </p>
        </div>
        <button
          onClick={() => setShowForm(!showForm)}
          className="inline-flex h-9 items-center rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground hover:bg-primary/90"
        >
          {showForm ? "Cancel" : "New Session"}
        </button>
      </div>

      {/* Create form */}
      {showForm && (
        <section className="rounded-xl border border-border bg-card p-6">
          <h2 className="text-base font-semibold">Create New Session</h2>
          <form onSubmit={handleCreate} className="mt-4 space-y-4">
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <div className="space-y-2">
                <label className="text-sm font-medium">Name</label>
                <input
                  type="text"
                  value={form.name}
                  onChange={(e) => setForm({ ...form, name: e.target.value })}
                  placeholder="Research Session"
                  required
                  className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                />
              </div>
              <div className="space-y-2">
                <label className="text-sm font-medium">LLM Provider</label>
                <select
                  value={form.provider_config_id ?? ""}
                  onChange={(e) =>
                    setForm({ ...form, provider_config_id: e.target.value || undefined })
                  }
                  className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                >
                  <option value="">System default</option>
                  {providers?.map((p) => (
                    <option key={p.id} value={p.id}>
                      {p.name} ({p.provider_type} / {p.default_model})
                    </option>
                  ))}
                </select>
              </div>
              <div className="space-y-2 sm:col-span-2">
                <label className="text-sm font-medium">Description</label>
                <input
                  type="text"
                  value={form.description ?? ""}
                  onChange={(e) => setForm({ ...form, description: e.target.value })}
                  placeholder="Optional description..."
                  className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                />
              </div>
            </div>
            <button
              type="submit"
              disabled={createMutation.isPending}
              className="inline-flex h-9 items-center rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
            >
              {createMutation.isPending ? "Creating..." : "Create Session"}
            </button>
          </form>
        </section>
      )}

      {/* Session list */}
      {isLoading ? (
        <div className="flex items-center justify-center py-12">
          <div className="h-6 w-6 animate-spin rounded-full border-2 border-primary border-t-transparent" />
        </div>
      ) : sessions.length === 0 ? (
        <div className="rounded-xl border border-dashed border-border p-12 text-center">
          <p className="text-sm text-muted-foreground">No sessions yet. Create one to get started.</p>
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-4">
          {sessions.map((session) => (
            <SessionCard
              key={session.id}
              session={session}
              isSelected={selectedSession === session.id}
              onSelect={() =>
                setSelectedSession(selectedSession === session.id ? null : session.id)
              }
              onArchive={() =>
                updateMutation.mutate({
                  id: session.id,
                  payload: { status: session.status === "archived" ? "active" : "archived" },
                })
              }
              onDelete={() => {
                if (confirm("Delete this session?")) deleteMutation.mutate(session.id);
              }}
              onCopyContext={() => setShowCopyDialog(session.id)}
              onSnapshot={() => setShowSnapshotDialog(session.id)}
              onChangeLLM={(providerConfigId) =>
                updateMutation.mutate({
                  id: session.id,
                  payload: { provider_config_id: providerConfigId },
                })
              }
              providers={providers ?? []}
              allSessions={sessions}
            />
          ))}
        </div>
      )}

      {/* Copy context dialog */}
      {showCopyDialog && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
          <div className="w-full max-w-md rounded-xl border border-border bg-card p-6 shadow-lg">
            <h3 className="text-base font-semibold">Copy Context From Another Session</h3>
            <p className="mt-1 text-xs text-muted-foreground">
              This will append the source session's messages to the target session.
            </p>
            <div className="mt-4 space-y-3">
              <select
                value={copySource}
                onChange={(e) => setCopySource(e.target.value)}
                className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
              >
                <option value="">Select source session...</option>
                {sessions
                  .filter((s) => s.id !== showCopyDialog)
                  .map((s) => (
                    <option key={s.id} value={s.id}>
                      {s.name} ({s.message_count} messages)
                    </option>
                  ))}
              </select>
            </div>
            <div className="mt-4 flex gap-2">
              <button
                onClick={() => handleCopyContext(showCopyDialog)}
                disabled={!copySource || copyMutation.isPending}
                className="inline-flex h-9 items-center rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
              >
                {copyMutation.isPending ? "Copying..." : "Copy Context"}
              </button>
              <button
                onClick={() => {
                  setShowCopyDialog(null);
                  setCopySource("");
                }}
                className="inline-flex h-9 items-center rounded-md border border-input bg-background px-4 text-sm font-medium hover:bg-accent"
              >
                Cancel
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Snapshot dialog */}
      {showSnapshotDialog && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
          <div className="w-full max-w-md rounded-xl border border-border bg-card p-6 shadow-lg">
            <h3 className="text-base font-semibold">Save Context Snapshot</h3>
            <p className="mt-1 text-xs text-muted-foreground">
              Save the current session state so you can restore it later.
            </p>
            <div className="mt-4 space-y-3">
              <input
                type="text"
                value={snapshotName}
                onChange={(e) => setSnapshotName(e.target.value)}
                placeholder="Snapshot name..."
                className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
              />
            </div>
            <div className="mt-4 flex gap-2">
              <button
                onClick={() => handleSnapshot(showSnapshotDialog)}
                disabled={!snapshotName || snapshotMutation.isPending}
                className="inline-flex h-9 items-center rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
              >
                {snapshotMutation.isPending ? "Saving..." : "Save Snapshot"}
              </button>
              <button
                onClick={() => {
                  setShowSnapshotDialog(null);
                  setSnapshotName("");
                }}
                className="inline-flex h-9 items-center rounded-md border border-input bg-background px-4 text-sm font-medium hover:bg-accent"
              >
                Cancel
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

// ─── Session Card Component ────────────────────────────────────────────────────

interface SessionCardProps {
  session: Session;
  isSelected: boolean;
  onSelect: () => void;
  onArchive: () => void;
  onDelete: () => void;
  onCopyContext: () => void;
  onSnapshot: () => void;
  onChangeLLM: (providerConfigId: string) => void;
  providers: { id: string; name: string; provider_type: string; default_model: string }[];
  allSessions: Session[];
}

function SessionCard({
  session,
  isSelected,
  onSelect,
  onArchive,
  onDelete,
  onCopyContext,
  onSnapshot,
  onChangeLLM,
  providers,
}: SessionCardProps) {
  const { data: snapshots } = useSessionSnapshots(isSelected ? session.id : "");
  const restoreMutation = useRestoreSnapshot();

  return (
    <div
      className={`rounded-xl border bg-card transition-colors ${
        isSelected ? "border-primary" : "border-border"
      }`}
    >
      <div className="p-5">
        <div className="flex items-start justify-between">
          <button onClick={onSelect} className="text-left">
            <div className="flex items-center gap-2">
              <h3 className="text-sm font-semibold">{session.name}</h3>
              <span
                className={`rounded-full px-2 py-0.5 text-xs font-medium ${
                  session.status === "active"
                    ? "bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400"
                    : "bg-gray-100 text-gray-700 dark:bg-gray-800 dark:text-gray-300"
                }`}
              >
                {session.status}
              </span>
            </div>
            {session.description && (
              <p className="mt-0.5 text-xs text-muted-foreground">{session.description}</p>
            )}
            <div className="mt-1 flex items-center gap-3 text-xs text-muted-foreground">
              <span>{session.message_count} messages</span>
              <span>{session.total_tokens.toLocaleString()} tokens</span>
              <span>{session.model_name ?? "default model"}</span>
              <span>Updated {new Date(session.updated_at).toLocaleString()}</span>
            </div>
          </button>

          <div className="flex items-center gap-1">
            <button
              onClick={onCopyContext}
              title="Copy context from another session"
              className="inline-flex h-8 items-center rounded-md border border-input bg-background px-2 text-xs font-medium hover:bg-accent"
            >
              Copy In
            </button>
            <button
              onClick={onSnapshot}
              title="Save a snapshot of current context"
              className="inline-flex h-8 items-center rounded-md border border-input bg-background px-2 text-xs font-medium hover:bg-accent"
            >
              Snapshot
            </button>
            <button
              onClick={onArchive}
              className="inline-flex h-8 items-center rounded-md border border-input bg-background px-2 text-xs font-medium hover:bg-accent"
            >
              {session.status === "archived" ? "Unarchive" : "Archive"}
            </button>
            <button
              onClick={onDelete}
              className="inline-flex h-8 items-center rounded-md border border-red-200 bg-background px-2 text-xs font-medium text-red-600 hover:bg-red-50 dark:border-red-800 dark:hover:bg-red-900/20"
            >
              Delete
            </button>
          </div>
        </div>

        {/* Expanded detail */}
        {isSelected && (
          <div className="mt-4 space-y-4 border-t border-border pt-4">
            {/* LLM switch */}
            <div className="flex items-center gap-3">
              <label className="text-xs font-medium">Switch LLM:</label>
              <select
                value={session.provider_config_id ?? ""}
                onChange={(e) => onChangeLLM(e.target.value)}
                className="flex h-8 rounded-md border border-input bg-background px-2 text-xs"
              >
                <option value="">System default</option>
                {providers.map((p) => (
                  <option key={p.id} value={p.id}>
                    {p.name} ({p.provider_type} / {p.default_model})
                  </option>
                ))}
              </select>
            </div>

            {/* Context messages preview */}
            {session.context_messages.length > 0 && (
              <div className="space-y-1">
                <p className="text-xs font-medium">Recent Context ({session.context_messages.length} messages)</p>
                <div className="max-h-48 overflow-y-auto rounded-md border border-border bg-background p-3">
                  {session.context_messages.slice(-5).map((msg, i) => (
                    <div key={i} className="mb-2 last:mb-0">
                      <span
                        className={`text-xs font-semibold ${
                          msg.role === "user"
                            ? "text-blue-600 dark:text-blue-400"
                            : msg.role === "assistant"
                            ? "text-green-600 dark:text-green-400"
                            : "text-gray-500"
                        }`}
                      >
                        {msg.role}
                        {msg.provider && ` (${msg.provider}/${msg.model})`}:
                      </span>
                      <p className="text-xs text-muted-foreground">{msg.content.slice(0, 200)}{msg.content.length > 200 ? "..." : ""}</p>
                    </div>
                  ))}
                </div>
              </div>
            )}

            {/* Snapshots */}
            {snapshots && snapshots.length > 0 && (
              <div className="space-y-2">
                <p className="text-xs font-medium">Snapshots</p>
                <div className="space-y-1">
                  {snapshots.map((snap) => (
                    <div
                      key={snap.id}
                      className="flex items-center justify-between rounded-md border border-border bg-background p-2"
                    >
                      <div>
                        <p className="text-xs font-medium">{snap.name}</p>
                        <p className="text-xs text-muted-foreground">
                          {snap.message_count} msgs, {snap.total_tokens} tokens &middot;{" "}
                          {new Date(snap.created_at).toLocaleString()}
                        </p>
                      </div>
                      <button
                        onClick={() =>
                          restoreMutation.mutate({
                            sessionId: session.id,
                            snapshotId: snap.id,
                          })
                        }
                        disabled={restoreMutation.isPending}
                        className="inline-flex h-7 items-center rounded-md bg-primary px-2 text-xs font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
                      >
                        Restore
                      </button>
                    </div>
                  ))}
                </div>
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  );
}
