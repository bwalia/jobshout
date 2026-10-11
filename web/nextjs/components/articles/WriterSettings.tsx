"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Copy, Loader2, Save, Settings2 } from "lucide-react";
import { toast } from "sonner";
import { cloneAgent } from "@/lib/api/agents";
import { getBlogDestinations } from "@/lib/api/blog";
import { useAgent, useUpdateAgent } from "@/lib/hooks/useAgents";
import { useAgentSchemas } from "@/lib/hooks/useAgentSchemas";
import { fieldOptions } from "@/lib/agents/input-schemas";
import { WRITER_SETTING_KEYS as K } from "@/lib/types/blog";

const ARTICLE_WRITER = "article_writer";

const inputClass =
  "mt-1 w-full rounded-md border border-border bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground";

function str(v: unknown): string {
  return typeof v === "string" ? v : "";
}

function focusText(v: unknown): string {
  if (Array.isArray(v)) return v.filter((x) => typeof x === "string").join(", ");
  return str(v);
}

/**
 * A writer agent's own settings: style, keywords, reader, website. This is how
 * the Article Writer is set up for a website or project without code — clone
 * it, then change the clone's settings here. Schedules and launches that name
 * their own keywords or reader still win over these defaults.
 */
export function WriterSettings({ agentId }: { agentId: string }) {
  const router = useRouter();
  const queryClient = useQueryClient();
  const { data: agent } = useAgent(agentId);
  const update = useUpdateAgent();
  const { data: catalog } = useAgentSchemas();
  const audiences = fieldOptions(catalog ?? [], ARTICLE_WRITER, "audience");
  const { data: destinations } = useQuery({
    queryKey: ["blog-destinations"],
    queryFn: getBlogDestinations,
  });

  const [open, setOpen] = useState(false);
  const [style, setStyle] = useState("");
  const [focus, setFocus] = useState("");
  const [industry, setIndustry] = useState("");
  const [reader, setReader] = useState("");
  const [destination, setDestination] = useState("");
  const [cloneName, setCloneName] = useState("");
  const [cloning, setCloning] = useState(false);

  useEffect(() => {
    const cfg = agent?.engine_config ?? {};
    setStyle(str(cfg[K.style]));
    setFocus(focusText(cfg[K.focus]));
    setIndustry(str(cfg[K.industry]));
    setReader(str(cfg[K.audience]));
    setDestination(str(cfg[K.destination]));
  }, [agent]);

  if (!agent) return null;

  function save() {
    if (!agent) return;
    const keywords = focus
      .split(",")
      .map((s) => s.trim())
      .filter(Boolean);
    update.mutate({
      id: agent.id,
      payload: {
        engine_config: {
          ...agent.engine_config,
          [K.style]: style.trim(),
          [K.focus]: keywords,
          [K.industry]: industry.trim(),
          [K.audience]: reader,
          [K.destination]: destination,
        },
      },
    });
  }

  async function clone() {
    if (!agent) return;
    const name = cloneName.trim();
    if (!name) return;
    setCloning(true);
    try {
      const copy = await cloneAgent(agent.id, name);
      await queryClient.invalidateQueries({ queryKey: ["agents"] });
      toast.success(`Cloned as "${copy.name}". Set its style, keywords and website.`);
      router.push(`/panel/agents/${copy.id}`);
    } catch (err) {
      toast.error(`Clone failed: ${err instanceof Error ? err.message : String(err)}`);
    } finally {
      setCloning(false);
    }
  }

  const selected = destinations?.find((d) => d.name === destination);

  return (
    <div className="rounded-lg border border-border bg-card">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        className="flex w-full items-center justify-between gap-3 px-4 py-3 text-left"
        aria-expanded={open}
      >
        <span className="flex items-center gap-2 text-sm font-medium text-foreground">
          <Settings2 className="h-4 w-4 text-muted-foreground" />
          Writer settings
        </span>
        <span className="truncate text-xs text-muted-foreground">
          {destination ? `Files to ${destination}` : "Files to this ring's CMS"}
          {focus ? ` · ${focus}` : ""}
        </span>
      </button>

      {open && (
        <div className="space-y-4 border-t border-border p-4">
          <div>
            <label htmlFor="writer-style" className="text-xs text-muted-foreground">
              Writing style
            </label>
            <textarea
              id="writer-style"
              rows={4}
              value={style}
              onChange={(e) => setStyle(e.target.value)}
              placeholder="e.g. Business and technology mix: lead with the benefit, then practical tips and tricks a team can try this week. 700–1,000 words, plain English, end with a short 'Try this' list."
              className={inputClass}
            />
          </div>

          <div className="grid gap-4 sm:grid-cols-2">
            <div>
              <label htmlFor="writer-focus" className="text-xs text-muted-foreground">
                Keywords
              </label>
              <input
                id="writer-focus"
                value={focus}
                onChange={(e) => setFocus(e.target.value)}
                placeholder="Agentic AI, FinTech, DevOps, SRE"
                className={inputClass}
              />
              <p className="mt-1 text-xs text-muted-foreground">
                Comma-separated. Used when a run picks its own trending topic.
              </p>
            </div>
            <div>
              <label htmlFor="writer-industry" className="text-xs text-muted-foreground">
                Industry
              </label>
              <input
                id="writer-industry"
                value={industry}
                onChange={(e) => setIndustry(e.target.value)}
                placeholder="Banking, HealthTech, Education"
                className={inputClass}
              />
            </div>
            <div>
              <label htmlFor="writer-reader" className="text-xs text-muted-foreground">
                Written for
              </label>
              <select
                id="writer-reader"
                value={reader}
                onChange={(e) => setReader(e.target.value)}
                className={inputClass}
              >
                <option value="">Default reader</option>
                {audiences.map((a) => (
                  <option key={a.value} value={a.value}>
                    {a.label}
                  </option>
                ))}
              </select>
            </div>
            <div>
              <label htmlFor="writer-destination" className="text-xs text-muted-foreground">
                Website
              </label>
              <select
                id="writer-destination"
                value={destination}
                onChange={(e) => setDestination(e.target.value)}
                className={inputClass}
              >
                <option value="">This ring&apos;s CMS</option>
                {destinations?.map((d) => (
                  <option key={d.name} value={d.name}>
                    {d.name}
                    {d.ready ? "" : " (not ready)"}
                  </option>
                ))}
                {destination && !selected && (
                  <option value={destination}>{destination} (missing here)</option>
                )}
              </select>
              <p className="mt-1 text-xs text-muted-foreground">
                {selected
                  ? `${selected.base_url} · ${selected.namespace}`
                  : destination
                    ? "No opsapi integration of this name in this organisation yet."
                    : "Add an opsapi integration to file to another website."}
              </p>
            </div>
          </div>

          <div className="flex flex-wrap items-end justify-between gap-3 border-t border-border pt-4">
            <div className="flex items-end gap-2">
              <div>
                <label htmlFor="clone-name" className="text-xs text-muted-foreground">
                  Clone for another website or project
                </label>
                <input
                  id="clone-name"
                  value={cloneName}
                  onChange={(e) => setCloneName(e.target.value)}
                  placeholder="e.g. Workstation Blog Writer"
                  className={inputClass}
                />
              </div>
              <button
                type="button"
                onClick={clone}
                disabled={!cloneName.trim() || cloning}
                className="inline-flex items-center gap-2 rounded-md border border-border px-3 py-2 text-sm text-foreground transition-colors hover:bg-muted disabled:opacity-50"
              >
                {cloning ? <Loader2 className="h-4 w-4 animate-spin" /> : <Copy className="h-4 w-4" />}
                Clone
              </button>
            </div>
            <button
              type="button"
              onClick={save}
              disabled={update.isPending}
              className="inline-flex items-center gap-2 rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 disabled:opacity-50"
            >
              {update.isPending ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4" />}
              Save settings
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
