"use client";

import Link from "next/link";
import { useEffect, useState, useTransition } from "react";
import { useFormState, useFormStatus } from "react-dom";
import { EMPTY_FORM_STATE } from "@/lib/form-state";
import { previewAction } from "@/app/insights/actions";
import { saveAppAction } from "@/app/showcase/actions";
import { CheckCircleIcon, ShieldIcon } from "@/components/icons";
import { LinkPicker } from "@/components/showcase/LinkPicker";
import type { Job } from "@/lib/api";
import { Button, ErrorNotice, Field, Input, Select, Textarea, buttonClass, cx } from "@/components/ui";
import {
  APP_TYPES,
  BUILD_METHODS,
  CAPABILITIES,
  EVIDENCE,
  KINDS,
  MATURITY,
  PRICING,
  VISIBILITY,
  type AppType,
  type BuildMethod,
  type Capability,
  type Kind,
  type Maturity,
  type Pricing,
  type ShowcaseApp,
  type Visibility,
} from "@/lib/showcase";

function SubmitButtons({ isStaff, live, noun }: { isStaff: boolean; live: boolean; noun: string }) {
  const { pending } = useFormStatus();
  return (
    <div className="flex flex-col-reverse gap-3 sm:flex-row sm:justify-end">
      {live ? null : (
        <Button type="submit" name="intent" value="draft" variant="secondary" size="lg" disabled={pending}>
          Save draft
        </Button>
      )}
      <Button type="submit" name="intent" value="submit" size="lg" disabled={pending}>
        {pending ? "Saving…" : live ? "Save changes" : isStaff ? `Publish ${noun}` : "Submit for review"}
      </Button>
    </div>
  );
}

function Section({ title, lead, children }: { title: string; lead?: string; children: React.ReactNode }) {
  return (
    <fieldset className="surface-card space-y-6 p-5 sm:p-7">
      <legend className="float-left mb-1 w-full font-display text-lg font-semibold text-ink">{title}</legend>
      {lead ? <p className="clear-both text-sm leading-relaxed text-mute">{lead}</p> : null}
      <div className="clear-both space-y-6">{children}</div>
    </fieldset>
  );
}

function RadioCards<T extends string>({
  name,
  label,
  options,
  value,
  onChange,
  columns = "sm:grid-cols-3",
}: {
  name: string;
  label: string;
  options: Array<{ value: T; label: string; blurb?: string }>;
  value: T | "";
  onChange: (v: T) => void;
  columns?: string;
}) {
  return (
    <div role="radiogroup" aria-label={label} className={cx("grid gap-2.5", columns)}>
      {options.map((o) => {
        const active = value === o.value;
        return (
          <label
            key={o.value}
            className={cx(
              "flex cursor-pointer flex-col gap-1 rounded-xl border p-3.5 transition-colors duration-200",
              "has-[:focus-visible]:outline has-[:focus-visible]:outline-2 has-[:focus-visible]:outline-offset-2 has-[:focus-visible]:outline-shout",
              active ? "border-shout bg-shout/[0.07]" : "border-line bg-surface hover:border-edge",
            )}
          >
            <input
              type="radio"
              name={name}
              value={o.value}
              checked={active}
              onChange={() => onChange(o.value)}
              className="sr-only"
            />
            <span className="text-sm font-semibold text-ink">{o.label}</span>
            {o.blurb ? <span className="text-xs leading-relaxed text-mute">{o.blurb}</span> : null}
          </label>
        );
      })}
    </div>
  );
}

function FieldError({ message }: { message?: string }) {
  return message ? (
    <p role="alert" className="mt-1.5 text-xs font-medium text-shout">
      {message}
    </p>
  ) : null;
}

export function AppForm({
  isStaff,
  initial,
  kind: newKind = "app",
  jobs = [],
}: {
  isStaff: boolean;
  initial?: ShowcaseApp | null;
  /** Open jobs the viewer may link (their own; editors: any). */
  jobs?: Job[];
  /** For a new entry; an existing one keeps its own kind. */
  kind?: Kind;
}) {
  const kind: Kind = initial?.kind ?? newKind;
  const noun = kind === "team" ? "team" : KINDS[kind].label.toLowerCase();
  const [state, action] = useFormState(saveAppAction, EMPTY_FORM_STATE);
  const [build, setBuild] = useState<BuildMethod | "">(initial?.build_method ?? "");
  const [maturity, setMaturity] = useState<Maturity | "">(initial?.maturity ?? "");
  const [visibility, setVisibility] = useState<Visibility>(initial?.visibility ?? "public");
  const [body, setBody] = useState(initial?.description_md ?? "");
  const [tab, setTab] = useState<"write" | "preview">("write");
  const [preview, setPreview] = useState("");
  const [previewing, startPreview] = useTransition();
  const live = initial?.status === "published";

  // Rendered by the API so the preview is exactly what will be published.
  useEffect(() => {
    if (tab !== "preview") return;
    const t = setTimeout(() => {
      startPreview(async () => {
        try {
          setPreview(await previewAction(body));
        } catch {
          setPreview("<p><em>Preview is unavailable right now.</em></p>");
        }
      });
    }, 150);
    return () => clearTimeout(t);
  }, [tab, body]);

  const errors = state.fieldErrors ?? {};
  const words = body.trim() ? body.trim().split(/\s+/).length : 0;
  const agentBuilt = build === "agent_built" || build === "agent_autonomous";
  const claimsProduction = maturity === "production_ready" || maturity === "enterprise_ready";

  if (state.ok && state.result?.id) {
    const status = state.result.status;
    return (
      <div className="surface-card animate-rise-in p-8 text-center sm:p-12">
        <span className="mx-auto flex h-14 w-14 items-center justify-center rounded-2xl bg-good/10 text-good">
          <CheckCircleIcon className="h-7 w-7" />
        </span>
        <h2 className="mt-5 font-display text-2xl font-semibold text-ink">
          {status === "published" ? "It is live" : status === "pending_review" ? "Submitted for review" : "Draft saved"}
        </h2>
        <p className="mx-auto mt-3 max-w-md text-sm leading-relaxed text-mute">
          {status === "published"
            ? kind === "app"
              ? "It is in the AI Showcase now."
              : "It is in the agent directory now."
            : status === "pending_review"
              ? `An editor checks every ${noun} and its links before it goes live. Track it, and any requested changes, under Your showcase.`
              : "Only you can see it. Come back and submit it when it is ready."}
        </p>
        <div className="mt-7 flex flex-wrap justify-center gap-3">
          <Link href={state.result.id} className={buttonClass("primary", "md")}>
            {status === "published" ? "View it" : "Preview it"}
          </Link>
          <Link href="/showcase/mine" className={buttonClass("secondary", "md")}>
            Your showcase
          </Link>
        </div>
      </div>
    );
  }

  return (
    <form action={action} className="space-y-8" noValidate>
      {initial ? <input type="hidden" name="id" value={initial.id} /> : null}
      <input type="hidden" name="kind" value={kind} />
      {state.message && !state.ok ? <ErrorNotice title={state.message} /> : null}

      <Section title={`The ${noun}`}>
        <div className={cx("grid gap-6", kind === "app" && "sm:grid-cols-[1fr_14rem]")}>
          <Field label="Name" htmlFor="name" required error={errors.name}>
            <Input id="name" name="name" defaultValue={initial?.name} maxLength={80} aria-invalid={Boolean(errors.name)} />
          </Field>
          {kind === "app" ? (
          <Field label="Kind of app" htmlFor="app_type" required error={errors.app_type}>
            <Select id="app_type" name="app_type" defaultValue={initial?.app_type ?? ""} aria-invalid={Boolean(errors.app_type)}>
              <option value="" disabled>
                Choose…
              </option>
              {(Object.keys(APP_TYPES) as AppType[]).map((t) => (
                <option key={t} value={t}>
                  {APP_TYPES[t]}
                </option>
              ))}
            </Select>
          </Field>
          ) : null}
        </div>
        <Field
          label="Tagline"
          htmlFor="tagline"
          required
          hint="One line: what it does, for whom. Shown on cards and in search."
          error={errors.tagline}
        >
          <Input id="tagline" name="tagline" defaultValue={initial?.tagline} maxLength={140} aria-invalid={Boolean(errors.tagline)} />
        </Field>

        <div>
          <div className="flex items-end justify-between gap-3">
            <label htmlFor="description_md" className="block text-sm font-semibold text-ink">
              Description<span className="ml-1 text-shout" aria-hidden>*</span>
            </label>
            <div role="tablist" aria-label="Editor mode" className="flex rounded-pill border border-line bg-raised p-0.5">
              {(["write", "preview"] as const).map((t) => (
                <button
                  key={t}
                  type="button"
                  role="tab"
                  aria-selected={tab === t}
                  aria-controls={`editor-${t}`}
                  onClick={() => setTab(t)}
                  className={cx(
                    "min-h-[32px] rounded-pill px-3.5 text-xs font-semibold capitalize transition-colors duration-200",
                    tab === t ? "bg-surface text-ink shadow-card" : "text-mute hover:text-ink",
                  )}
                >
                  {t}
                </button>
              ))}
            </div>
          </div>
          <p id="description_md-hint" className="mt-1 text-xs leading-relaxed text-mute">
            Markdown. What it does, who it is for, and how it works. At least 20 words to submit — {words} so far.
          </p>
          <div className="mt-2">
            <div id="editor-write" role="tabpanel" hidden={tab !== "write"}>
              <Textarea
                id="description_md"
                name="description_md"
                rows={12}
                value={body}
                onChange={(e) => setBody(e.target.value)}
                aria-describedby="description_md-hint"
                aria-invalid={Boolean(errors.description_md)}
                className="font-mono text-[0.9rem]"
                placeholder={"## What it does\n\n## How it works\n\n## What's next"}
              />
            </div>
            <div
              id="editor-preview"
              role="tabpanel"
              hidden={tab !== "preview"}
              aria-busy={previewing}
              className="min-h-[12rem] rounded-xl border border-line bg-surface px-5 py-4"
            >
              {preview ? (
                <div className="insight-prose" dangerouslySetInnerHTML={{ __html: preview }} />
              ) : (
                <p className="text-sm text-mute">{previewing ? "Rendering…" : "Nothing to preview yet."}</p>
              )}
            </div>
          </div>
          <FieldError message={errors.description_md} />
        </div>
      </Section>

      <Section
        title="Links"
        lead={
          live
            ? `Changing a link, the logo or screenshots sends the ${noun} back to an editor before the change goes live.`
            : kind === "app"
              ? "At least one of repository, demo or website. Visitors leave JobShout through these, so an editor checks them."
              : "Visitors leave JobShout through these, so an editor checks them."
        }
      >
        <div className="grid gap-6 sm:grid-cols-2">
          {(
            [
              ["repo_url", "Repository", "GitHub, GitLab, Bitbucket…"],
              ["demo_url", "Live demo", "Where people can try it."],
              ["website_url", "Website", undefined],
              ["docs_url", "Documentation", undefined],
            ] as const
          ).map(([key, label, hint]) => (
            <Field key={key} label={label} htmlFor={key} hint={hint} error={errors[key]}>
              <Input
                id={key}
                name={key}
                type="url"
                inputMode="url"
                placeholder="https://"
                defaultValue={initial?.[key]}
                aria-invalid={Boolean(errors[key])}
              />
            </Field>
          ))}
        </div>
        <Field label={kind === "app" ? "Logo URL" : "Avatar URL"} htmlFor="logo_url" hint="Square works best. Leave empty for a lettered tile." error={errors.logo_url}>
          <Input id="logo_url" name="logo_url" type="url" inputMode="url" placeholder="https://" defaultValue={initial?.logo_url} />
        </Field>
        {kind === "app" ? (
        <Field label="Screenshots" htmlFor="screenshots" hint="Up to eight image links, one per line." error={errors.screenshots}>
          <Textarea
            id="screenshots"
            name="screenshots"
            rows={3}
            defaultValue={initial?.screenshots.join("\n")}
            placeholder="https://"
            className="font-mono text-[0.85rem]"
          />
        </Field>
        ) : null}
      </Section>

      {kind === "agent" ? (
        <Section title="What it runs on and can do">
          <div className="grid gap-6 sm:grid-cols-2">
            <Field label="Model" htmlFor="ai_models" required hint="Comma-separated if it uses more than one." error={errors.ai_models}>
              <Input id="ai_models" name="ai_models" defaultValue={initial?.ai_models.join(", ")} placeholder="Claude" />
            </Field>
            <Field label="Model provider" htmlFor="model_provider" error={errors.model_provider}>
              <Input id="model_provider" name="model_provider" defaultValue={initial?.model_provider} maxLength={80} placeholder="Anthropic" />
            </Field>
          </div>
          <div>
            <p className="text-sm font-semibold text-ink" id="capabilities-label">
              Capabilities<span className="ml-1 text-shout" aria-hidden>*</span>
            </p>
            <div role="group" aria-labelledby="capabilities-label" className="mt-3 flex flex-wrap gap-2">
              {(Object.keys(CAPABILITIES) as Capability[]).map((c) => (
                <label
                  key={c}
                  className="inline-flex min-h-[40px] cursor-pointer items-center gap-2 rounded-pill border border-line px-3.5 text-sm text-body transition-colors duration-200 hover:border-edge has-[:checked]:border-shout/50 has-[:checked]:bg-shout/10 has-[:checked]:font-semibold has-[:checked]:text-ink has-[:focus-visible]:outline has-[:focus-visible]:outline-2 has-[:focus-visible]:outline-shout"
                >
                  <input type="checkbox" name="capabilities" value={c} defaultChecked={initial?.capabilities.includes(c)} className="sr-only" />
                  {CAPABILITIES[c]}
                </label>
              ))}
            </div>
            <FieldError message={errors.capabilities} />
          </div>
          <Field label="Skills" htmlFor="technologies" hint="Languages, frameworks and domains. Comma-separated, up to 15." error={errors.technologies}>
            <Input id="technologies" name="technologies" defaultValue={initial?.technologies.join(", ")} placeholder="Rust, Axum, Kubernetes" />
          </Field>
          <div className="grid gap-6 sm:grid-cols-2">
            <Field label="Tools" htmlFor="tools" hint="Comma-separated, up to 20." error={errors.tools}>
              <Input id="tools" name="tools" defaultValue={initial?.tools.join(", ")} placeholder="GitHub, Docker" />
            </Field>
            <Field label="MCP servers" htmlFor="mcp_servers" hint="Comma-separated, up to 15." error={errors.mcp_servers}>
              <Input id="mcp_servers" name="mcp_servers" defaultValue={initial?.mcp_servers.join(", ")} placeholder="GitHub, Filesystem" />
            </Field>
          </div>
          <Field
            label="Human oversight"
            htmlFor="human_oversight"
            hint="Where people review or approve its work. No prompts or secrets needed."
            error={errors.human_oversight}
          >
            <Textarea id="human_oversight" name="human_oversight" rows={2} maxLength={1000} defaultValue={initial?.human_oversight} />
          </Field>
        </Section>
      ) : null}

      {kind === "team" ? (
        <Section title="Members" lead="At least two agents from the directory, in the order the work flows between them.">
          <LinkPicker
            name="agent_links"
            kind="agent"
            label="Agents"
            hint="Give each one its role in the team."
            initial={initial?.linked_agents ?? []}
            error={errors.agent_links}
          />
          <Field
            label="Human oversight"
            htmlFor="human_oversight"
            hint="Where people review or approve the team's work."
            error={errors.human_oversight}
          >
            <Textarea id="human_oversight" name="human_oversight" rows={2} maxLength={1000} defaultValue={initial?.human_oversight} />
          </Field>
        </Section>
      ) : null}

      {kind === "app" ? (
      <Section title="How it was built" lead="Say who did the work. JobShout shows this as the creator's account, not as something it has checked.">
        <div>
          <p className="text-sm font-semibold text-ink">
            Built by<span className="ml-1 text-shout" aria-hidden>*</span>
          </p>
          <div className="mt-3">
            <RadioCards
              name="build_method"
              label="Built by"
              value={build}
              onChange={setBuild}
              options={(Object.keys(BUILD_METHODS) as BuildMethod[]).map((b) => ({
                value: b,
                label: BUILD_METHODS[b].label,
                blurb: BUILD_METHODS[b].blurb,
              }))}
            />
          </div>
          <FieldError message={errors.build_method} />
        </div>
        <LinkPicker
          name="agent_links"
          kind="agent"
          label="Agents from the directory"
          hint={`${agentBuilt ? "Name at least one agent here, a team, or below. " : ""}Their profiles link back to this app.`}
          initial={initial?.linked_agents ?? []}
          error={errors.agent_links}
        />
        <LinkPicker
          name="team_slug"
          kind="team"
          label="Agent team"
          single
          roles={false}
          initial={initial?.linked_team ? [initial.linked_team] : []}
          error={errors.team_slug}
        />
        <Field
          label="Other agents"
          htmlFor="agents"
          hint="Agents not in the directory. One per line: name — what it did."
          error={errors.agents}
        >
          <Textarea
            id="agents"
            name="agents"
            rows={4}
            defaultValue={initial?.agents.map((a) => (a.role ? `${a.name} — ${a.role}` : a.name)).join("\n")}
            aria-invalid={Boolean(errors.agents)}
          />
        </Field>
        <div className="grid gap-6 sm:grid-cols-2">
          <Field label="AI models" htmlFor="ai_models" hint="Comma-separated." error={errors.ai_models}>
            <Input id="ai_models" name="ai_models" defaultValue={initial?.ai_models.join(", ")} placeholder="Claude, an open-weight model" />
          </Field>
          <Field label="Technologies" htmlFor="technologies" hint="Comma-separated, up to 15." error={errors.technologies}>
            <Input id="technologies" name="technologies" defaultValue={initial?.technologies.join(", ")} placeholder="Rust, PostgreSQL, MCP" />
          </Field>
        </div>
        <Field
          label="Human oversight"
          htmlFor="human_oversight"
          hint="Where people reviewed or approved the agents' work. No prompts or secrets needed."
          error={errors.human_oversight}
        >
          <Textarea id="human_oversight" name="human_oversight" rows={2} maxLength={1000} defaultValue={initial?.human_oversight} />
        </Field>
      </Section>
      ) : null}

      {kind === "app" ? (
      <Section title="Maturity">
        <div>
          <p className="text-sm font-semibold text-ink">
            How far along is it?<span className="ml-1 text-shout" aria-hidden>*</span>
          </p>
          <div className="mt-3">
            <RadioCards
              name="maturity"
              label="Maturity"
              value={maturity}
              onChange={setMaturity}
              columns="grid-cols-2 sm:grid-cols-4"
              options={(Object.keys(MATURITY) as Maturity[]).map((m) => ({ value: m, label: MATURITY[m].label }))}
            />
          </div>
          <p className="mt-2 text-sm text-mute">{maturity ? MATURITY[maturity].blurb : "Pick the stage that is true today."}</p>
          <FieldError message={errors.maturity} />
        </div>

        <div
          className={cx(
            "rounded-xl border p-4 sm:p-5",
            claimsProduction ? "border-shout/35 bg-shout/[0.04]" : "border-line",
          )}
        >
          <p className="flex items-center gap-2 text-sm font-semibold text-ink">
            <ShieldIcon className="h-4 w-4 text-mute" />
            Production evidence
          </p>
          <p className="mt-1 text-xs leading-relaxed text-mute">
            {claimsProduction
              ? "Production ready needs tests, CI/CD, security scanning, monitoring, documentation and a live demo or website. Enterprise ready also needs dependency scanning, backups and a release history."
              : "Optional below production ready. Tick what is true; it shows on your page as self-declared."}
          </p>
          <div className="mt-3 grid gap-x-6 sm:grid-cols-2">
            {EVIDENCE.map(({ key, label }) => (
              <label key={key} className="flex min-h-[40px] cursor-pointer items-center gap-3 text-sm text-body">
                <input
                  type="checkbox"
                  name={`evidence_${key}`}
                  defaultChecked={initial?.evidence[key]}
                  className="h-5 w-5 cursor-pointer rounded-md border-line accent-[rgb(var(--brand))]"
                />
                {label}
              </label>
            ))}
          </div>
          <div className="mt-3">
            <Field label="Status or uptime page" htmlFor="status_page_url" error={errors.status_page_url}>
              <Input
                id="status_page_url"
                name="status_page_url"
                type="url"
                inputMode="url"
                placeholder="https://"
                defaultValue={initial?.evidence.status_page_url}
              />
            </Field>
          </div>
          <FieldError message={errors.evidence} />
        </div>
      </Section>
      ) : null}

      <Section
        title="Open roles"
        lead={`Hiring for this ${noun}? Link roles you have posted on the board; they show on its page and it appears under Hiring.`}
      >
        {(() => {
          // Roles already linked stay listed even if someone else posted them.
          const options = [...(initial?.jobs ?? []), ...jobs.filter((j) => !initial?.jobs.some((x) => x.id === j.id))];
          return options.length ? (
            <div role="group" aria-label="Open roles" className="space-y-2">
              {options.map((j) => (
                <label
                  key={j.id}
                  className="flex min-h-[44px] cursor-pointer items-center gap-3 rounded-xl border border-line px-3.5 py-2.5 text-sm transition-colors duration-200 hover:border-edge has-[:checked]:border-shout/50 has-[:checked]:bg-shout/[0.06]"
                >
                  <input
                    type="checkbox"
                    name="job_ids"
                    value={j.id}
                    defaultChecked={initial?.jobs.some((x) => x.id === j.id)}
                    className="h-5 w-5 cursor-pointer rounded-md accent-[rgb(var(--brand))]"
                  />
                  <span className="min-w-0">
                    <span className="block truncate font-semibold text-ink">{j.title}</span>
                    <span className="block truncate text-xs text-mute">{j.summary || j.location.country}</span>
                  </span>
                </label>
              ))}
            </div>
          ) : (
            <p className="text-sm text-mute">
              You have no open roles on the board.{" "}
              <Link href="/post-job" className="font-medium text-ink underline decoration-shout/50 underline-offset-4 hover:text-shout">
                Post a job
              </Link>{" "}
              while signed in, then link it here.
            </p>
          );
        })()}
        <FieldError message={errors.job_ids} />
      </Section>

      <Section title="Details">
        <div className="grid gap-6 sm:grid-cols-3">
          <Field label="Pricing" htmlFor="pricing">
            <Select id="pricing" name="pricing" defaultValue={initial?.pricing ?? "free"}>
              {(Object.keys(PRICING) as Pricing[]).map((p) => (
                <option key={p} value={p}>
                  {PRICING[p]}
                </option>
              ))}
            </Select>
          </Field>
          <Field label="Licence" htmlFor="license" error={errors.license}>
            <Input id="license" name="license" defaultValue={initial?.license} maxLength={80} placeholder="MIT" />
          </Field>
          <Field label="Version" htmlFor="version" error={errors.version}>
            <Input id="version" name="version" defaultValue={initial?.version} maxLength={80} placeholder="1.0.0" />
          </Field>
        </div>
        {kind !== "team" ? (
          <Field label="Team or organisation" htmlFor="team_name" hint="Shown instead of your name on cards." error={errors.team_name}>
            <Input id="team_name" name="team_name" defaultValue={initial?.team_name} maxLength={80} />
          </Field>
        ) : null}
        <div>
          <p className="text-sm font-semibold text-ink">Who can see it</p>
          <div className="mt-3">
            <RadioCards
              name="visibility"
              label="Visibility"
              value={visibility}
              onChange={setVisibility}
              options={(Object.keys(VISIBILITY) as Visibility[]).map((v) => ({
                value: v,
                label: VISIBILITY[v].label,
                blurb: VISIBILITY[v].blurb,
              }))}
            />
          </div>
        </div>
      </Section>

      {!isStaff ? (
        <p className="text-sm leading-relaxed text-mute">
          An editor reviews every {noun} before it goes live. Only list work you have the right to show, and do not
          paste credentials, private prompts or customer data.
        </p>
      ) : null}

      <SubmitButtons isStaff={isStaff} live={live} noun={noun} />
    </form>
  );
}
