"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
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
import { AddWebsiteForm } from "@/components/blog/AddWebsiteForm";

const ARTICLE_WRITER = "article_writer";

/** The website picker's "add one" entry; never a real integration name. */
const ADD_WEBSITE = "\u0000add";

const inputClass =
  "mt-1 w-full rounded-md border border-border bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground";

function str(v: unknown): string {
  return typeof v === "string" ? v : "";
}

function focusText(v: unknown): string {
  if (Array.isArray(v)) return v.filter((x) => typeof x === "string").join(", ");
  return str(v);
}

function list(text: string): string[] {
  return text
    .split(",")
    .map((s) => s.trim())
    .filter(Boolean);
}

/**
 * A writer agent's own settings: style, content rules, keywords, reader,
 * website, and how posts are filed there (byline, categories, tags, images).
 * This is how
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
  const [rules, setRules] = useState("");
  const [author, setAuthor] = useState("");
  const [categories, setCategories] = useState("");
  const [tags, setTags] = useState("");
  const [cmsImages, setCmsImages] = useState(false);
  const [addingSite, setAddingSite] = useState(false);
  const [cloneName, setCloneName] = useState("");
  const [cloning, setCloning] = useState(false);

  useEffect(() => {
    const cfg = agent?.engine_config ?? {};
    setStyle(str(cfg[K.style]));
    setFocus(focusText(cfg[K.focus]));
    setIndustry(str(cfg[K.industry]));
    setReader(str(cfg[K.audience]));
    setDestination(str(cfg[K.destination]));
    setRules(str(cfg[K.rules]));
    setAuthor(str(cfg[K.author]));
    setCategories(focusText(cfg[K.categories]));
    setTags(focusText(cfg[K.tags]));
    setCmsImages(cfg[K.cmsImages] === true || cfg[K.cmsImages] === "true");
  }, [agent]);

  if (!agent) return null;

  // site overrides the picked website: a website just added is saved at once.
  function save(site = destination) {
    if (!agent) return;
    const keywords = list(focus);
    update.mutate({
      id: agent.id,
      payload: {
        engine_config: {
          ...agent.engine_config,
          [K.style]: style.trim(),
          [K.focus]: keywords,
          [K.industry]: industry.trim(),
          [K.audience]: reader,
          [K.destination]: site,
          [K.rules]: rules.trim(),
          [K.author]: author.trim(),
          [K.categories]: list(categories),
          [K.tags]: list(tags),
          [K.cmsImages]: cmsImages,
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
          {destination ? `Files to ${destination}` : "Files to the default website"}
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

          <div>
            <label htmlFor="writer-rules" className="text-xs text-muted-foreground">
              Content rules
            </label>
            <textarea
              id="writer-rules"
              rows={4}
              value={rules}
              onChange={(e) => setRules(e.target.value)}
              placeholder="e.g. Never give financial or investment advice. Never promise or imply a return. Any figure must be dated and sourced."
              className={inputClass}
            />
            <p className="mt-1 text-xs text-muted-foreground">
              Stricter than the style: the writer checks its draft against these and fixes every break. One
              rule per line.
            </p>
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
                value={addingSite ? ADD_WEBSITE : destination}
                onChange={(e) => {
                  if (e.target.value === ADD_WEBSITE) {
                    setAddingSite(true);
                    return;
                  }
                  setAddingSite(false);
                  setDestination(e.target.value);
                }}
                className={inputClass}
              >
                <option value="">Default website (set on the server)</option>
                {destinations?.map((d) => (
                  <option key={d.name} value={d.name}>
                    {d.name}
                    {d.ready ? "" : " (not ready)"}
                  </option>
                ))}
                {destination && !selected && (
                  <option value={destination}>{destination} (missing here)</option>
                )}
                <option value={ADD_WEBSITE}>+ Add a website…</option>
              </select>
              <p className="mt-1 text-xs text-muted-foreground">
                {selected ? (
                  `${selected.base_url} · ${selected.namespace}`
                ) : (
                  <>
                    {destination
                      ? "No website of this name is set up in this organisation yet. "
                      : "Add an OpsAPI namespace and its key with + Add a website…. "}
                    <Link href="/settings#integrations" className="underline hover:text-foreground">
                      Manage integrations
                    </Link>
                  </>
                )}
              </p>
            </div>
          </div>

          <fieldset className="grid gap-4 border-t border-border pt-4 sm:grid-cols-2">
            <legend className="mb-2 text-xs font-medium text-foreground">How posts are filed on the website</legend>
            <div>
              <label htmlFor="writer-author" className="text-xs text-muted-foreground">
                Byline
              </label>
              <input
                id="writer-author"
                value={author}
                onChange={(e) => setAuthor(e.target.value)}
                placeholder="Default byline (set on the server)"
                className={inputClass}
              />
            </div>
            <div>
              <label htmlFor="writer-categories" className="text-xs text-muted-foreground">
                Categories
              </label>
              <input
                id="writer-categories"
                value={categories}
                onChange={(e) => setCategories(e.target.value)}
                placeholder="Market insights"
                className={inputClass}
              />
              <p className="mt-1 text-xs text-muted-foreground">
                Comma-separated names or slugs that already exist on the website. The first is the main one.
              </p>
            </div>
            <div>
              <label htmlFor="writer-tags" className="text-xs text-muted-foreground">
                Tags
              </label>
              <input
                id="writer-tags"
                value={tags}
                onChange={(e) => setTags(e.target.value)}
                placeholder="Tags for who it was written for"
                className={inputClass}
              />
              <p className="mt-1 text-xs text-muted-foreground">
                Comma-separated. Replaces the default tags; readers see these.
              </p>
            </div>
            <label className="flex items-start gap-2 self-center text-sm text-foreground">
              <input
                type="checkbox"
                checked={cmsImages}
                onChange={(e) => setCmsImages(e.target.checked)}
                className="mt-1"
              />
              <span>
                Store images on the website
                <span className="block text-xs text-muted-foreground">
                  Copies the cover and diagrams to the website&apos;s media library, so the site does not load
                  them from JobShout.
                </span>
              </span>
            </label>
          </fieldset>

          {addingSite && (
            <AddWebsiteForm
              takenNames={(destinations ?? []).map((d) => d.name.toLowerCase())}
              onSaved={(name) => {
                setDestination(name);
                setAddingSite(false);
                save(name);
              }}
              onCancel={() => setAddingSite(false)}
            />
          )}

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
              onClick={() => save()}
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
