"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { getBlogDestinations } from "@/lib/api/blog";
import { useBlogConfig, useGenerateBlog } from "@/lib/hooks/useBlog";
import { useAgent } from "@/lib/hooks/useAgents";
import { useAgentSchemas } from "@/lib/hooks/useAgentSchemas";
import { fieldDefault, fieldOptions } from "@/lib/agents/input-schemas";
import { WRITER_SETTING_KEYS, type BlogBrief } from "@/lib/types/blog";
import { AddWebsiteForm } from "@/components/blog/AddWebsiteForm";

/**
 * The Article Writer's builtin marker. Its audience choices come from
 * GET /agent-schemas rather than a list kept here — one schema, in Go, which
 * the web reads. See .claude/rules/agent-modules.md.
 */
const ARTICLE_WRITER = "article_writer";

/** The website picker's "add one" entry; never a real integration name. */
const ADD_WEBSITE = "\u0000add";

/** Matches blog.HardMaxArticles on the server, which truncates beyond this. */
const HARD_MAX_ARTICLES = 10;

/** A brief plus the local id React needs to keep inputs stable across edits. */
interface BriefRow extends BlogBrief {
  id: number;
}

let nextRowID = 0;
function newRow(): BriefRow {
  return { id: nextRowID++, topic: "", context: "" };
}

/**
 * Briefs the Article Writer. One box per article: a topic to research and
 * optional guidance shaping how it gets written.
 *
 * This replaced a single textarea split on newlines. That form could only
 * express a list of subjects — there was nowhere to say who the piece is for or
 * what to avoid, and a topic containing a line break silently became two
 * articles.
 */
export function GenerateArticleDialog({
  open,
  onClose,
  agentId,
}: {
  open: boolean;
  onClose: () => void;
  /** The writer agent to run as — the original or a clone. */
  agentId?: string;
}) {
  const router = useRouter();
  const generate = useGenerateBlog();
  const [rows, setRows] = useState<BriefRow[]>([newRow()]);
  const [model, setModel] = useState("");
  // The reader and sector apply to the whole run: one dialog writes one kind
  // of piece, and a per-row picker would be a choice nobody wants to make five
  // times. A brief can still override it over the API.
  const [runAudience, setRunAudience] = useState("");
  const [industry, setIndustry] = useState("");
  // The OpsAPI website (integration name) to file in; "" is the writer's own
  // website, else this ring's CMS.
  const [destination, setDestination] = useState("");
  const [addingSite, setAddingSite] = useState(false);

  const { data: config } = useBlogConfig();
  const { data: agent } = useAgent(agentId ?? "");
  const { data: destinations } = useQuery({
    queryKey: ["blog-destinations"],
    queryFn: getBlogDestinations,
    enabled: open,
  });
  const sites = destinations ?? [];
  const agentSite =
    typeof agent?.engine_config?.[WRITER_SETTING_KEYS.destination] === "string"
      ? (agent.engine_config[WRITER_SETTING_KEYS.destination] as string)
      : "";
  const defaultLabel = agentSite
    ? `Writer's website (${agentSite})`
    : config?.cms_namespace
      ? `Default (${config.cms_namespace})`
      : "Default (no CMS set up on this ring)";

  const { data: agentCatalog } = useAgentSchemas();
  const catalog = agentCatalog ?? [];
  const audiences = fieldOptions(catalog, ARTICLE_WRITER, "audience");
  const defaultAudience = fieldDefault(catalog, ARTICLE_WRITER, "audience");

  const filled = rows.filter((r) => r.topic.trim() !== "");
  const atCap = rows.length >= HARD_MAX_ARTICLES;
  const canSubmit = filled.length > 0 && !generate.isPending && !addingSite;

  if (!open) return null;

  function updateRow(id: number, patch: Partial<BlogBrief>) {
    setRows((prev) =>
      prev.map((r) => (r.id === id ? { ...r, ...patch } : r))
    );
  }

  function addRow() {
    setRows((prev) => [...prev, newRow()]);
  }

  function removeRow(id: number) {
    // Never drop to zero rows — an empty dialog gives the user nothing to type
    // into and no obvious way back.
    setRows((prev) => (prev.length === 1 ? prev : prev.filter((r) => r.id !== id)));
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!canSubmit) return;

    const briefs: BlogBrief[] = filled.map((r) => ({
      topic: r.topic.trim(),
      context: r.context?.trim() || undefined,
    }));

    generate.mutate(
      {
        briefs,
        model: model.trim() || undefined,
        audience: runAudience || undefined,
        industry: industry.trim() || undefined,
        agent_id: agentId,
        destination: destination || undefined,
      },
      {
        onSuccess: (run) => {
          setRows([newRow()]);
          setModel("");
          setRunAudience("");
          setIndustry("");
          setDestination("");
          setAddingSite(false);
          onClose();
          // Go straight to the run so the user watches it work rather than
          // wondering whether anything happened.
          router.push(`/articles/${run.id}`);
        },
      }
    );
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4 backdrop-blur-sm">
      <form
        onSubmit={handleSubmit}
        className="flex max-h-[90vh] w-full max-w-2xl flex-col rounded-lg border border-border bg-card shadow-xl"
      >
        <div className="border-b border-border p-6 pb-4">
          <h2 className="text-lg font-semibold text-foreground">
            Write articles
          </h2>
          <p className="mt-1 text-sm text-muted-foreground">
            Give each article a topic to research. The writer finds current
            sources, chooses its own title from what it learns, and cites what it
            used. Nothing reaches the CMS until you say so.
          </p>
        </div>

        <div className="flex-1 space-y-4 overflow-y-auto p-6">
          <div className="grid gap-4 sm:grid-cols-2">
            <div>
              <label htmlFor="audience" className="text-xs text-muted-foreground">
                Written for
              </label>
              <select
                id="audience"
                value={runAudience || defaultAudience}
                onChange={(e) =>
                  setRunAudience(
                    e.target.value === defaultAudience ? "" : e.target.value
                  )
                }
                disabled={audiences.length === 0}
                className="mt-1 w-full rounded-md border border-border bg-background px-3 py-2 text-sm text-foreground disabled:opacity-60"
              >
                {audiences.length === 0 ? (
                  <option value="">Loading audiences…</option>
                ) : (
                  audiences.map((a) => (
                    <option key={a.value} value={a.value}>
                      {a.label}
                    </option>
                  ))
                )}
              </select>
            </div>
            <div>
              <label htmlFor="industry" className="text-xs text-muted-foreground">
                Industry{" "}
                <span className="text-muted-foreground">(optional)</span>
              </label>
              <input
                id="industry"
                value={industry}
                onChange={(e) => setIndustry(e.target.value)}
                placeholder="NHS trusts, 3PL logistics"
                className="mt-1 w-full rounded-md border border-border bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground"
              />
            </div>
            <div className="sm:col-span-2">
              <label htmlFor="destination" className="text-xs text-muted-foreground">
                Website
              </label>
              <select
                id="destination"
                value={addingSite ? ADD_WEBSITE : destination}
                onChange={(e) => {
                  if (e.target.value === ADD_WEBSITE) {
                    setAddingSite(true);
                    return;
                  }
                  setAddingSite(false);
                  setDestination(e.target.value);
                }}
                className="mt-1 w-full rounded-md border border-border bg-background px-3 py-2 text-sm text-foreground"
              >
                <option value="">{defaultLabel}</option>
                {sites.map((d) => (
                  <option key={d.name} value={d.name} disabled={!d.ready}>
                    {d.name} — {d.namespace || "no namespace"}
                    {d.ready ? "" : " (needs its key or namespace)"}
                  </option>
                ))}
                <option value={ADD_WEBSITE}>+ Add a website…</option>
              </select>
              {sites.length === 0 && !addingSite && (
                <p className="mt-1 text-2xs text-muted-foreground">
                  Publishing to another OpsAPI namespace, such as jobshout? Add
                  it once and every later article can pick it.
                </p>
              )}
            </div>
            {addingSite && (
              <div className="sm:col-span-2">
                <AddWebsiteForm
                  takenNames={sites.map((d) => d.name.toLowerCase())}
                  onSaved={(name) => {
                    setDestination(name);
                    setAddingSite(false);
                  }}
                  onCancel={() => setAddingSite(false)}
                />
              </div>
            )}
            <p className="text-2xs text-muted-foreground sm:col-span-2">
              This applies to every article below. It changes how each one is
              researched, structured, written and reviewed — a business briefing
              is not a developer article with simpler words.
            </p>
          </div>

          {rows.map((row, i) => (
            <div
              key={row.id}
              className="rounded-md border border-border bg-background/40 p-4"
            >
              <div className="flex items-center justify-between">
                <span className="text-2xs font-medium uppercase tracking-wide text-muted-foreground">
                  Article {i + 1}
                </span>
                {rows.length > 1 && (
                  <button
                    type="button"
                    onClick={() => removeRow(row.id)}
                    className="text-2xs text-muted-foreground transition-colors hover:text-destructive"
                    aria-label={`Remove article ${i + 1}`}
                  >
                    Remove
                  </button>
                )}
              </div>

              <label
                htmlFor={`topic-${row.id}`}
                className="mt-3 block text-xs text-muted-foreground"
              >
                Topic
              </label>
              <input
                id={`topic-${row.id}`}
                autoFocus={i === 0}
                value={row.topic}
                onChange={(e) => updateRow(row.id, { topic: e.target.value })}
                placeholder="Kubernetes Gateway API replacing Ingress"
                className="mt-1 w-full rounded-md border border-border bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground"
              />

              <label
                htmlFor={`context-${row.id}`}
                className="mt-3 block text-xs text-muted-foreground"
              >
                Context{" "}
                <span className="text-muted-foreground">(optional)</span>
              </label>
              <textarea
                id={`context-${row.id}`}
                rows={2}
                value={row.context ?? ""}
                onChange={(e) => updateRow(row.id, { context: e.target.value })}
                placeholder="Angle, points to cover, things to avoid"
                className="mt-1 w-full rounded-md border border-border bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground"
              />
            </div>
          ))}

          <button
            type="button"
            onClick={addRow}
            disabled={atCap}
            className="w-full rounded-md border border-dashed border-border py-2 text-sm text-muted-foreground transition-colors hover:bg-accent disabled:cursor-not-allowed disabled:opacity-50"
          >
            + Add another article
          </button>

          <div>
            <label htmlFor="model" className="text-xs text-muted-foreground">
              Model <span className="text-muted-foreground">(optional)</span>
            </label>
            <input
              id="model"
              value={model}
              onChange={(e) => setModel(e.target.value)}
              placeholder="Leave blank to use the configured default"
              className="mt-1 w-full rounded-md border border-border bg-background px-3 py-1.5 text-sm text-foreground placeholder:text-muted-foreground"
            />
          </div>
        </div>

        <div className="flex items-center justify-between gap-2 border-t border-border p-6 pt-4">
          <p className="text-2xs text-muted-foreground">
            {filled.length} article{filled.length === 1 ? "" : "s"}
            {atCap && (
              <span>
                {" "}
                — maximum is {HARD_MAX_ARTICLES}
              </span>
            )}
          </p>
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={onClose}
              className="rounded-md border border-border px-4 py-2 text-sm text-muted-foreground transition-colors hover:bg-accent"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={!canSubmit}
              className="rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 disabled:opacity-50"
            >
              {generate.isPending ? "Starting..." : "Write"}
            </button>
          </div>
        </div>
      </form>
    </div>
  );
}
