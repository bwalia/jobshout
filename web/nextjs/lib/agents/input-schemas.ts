import type { Agent } from "@/lib/types/agent";

/** Field types the task-manager forms can render. */
export type AgentFieldType =
  | "text"
  | "textarea"
  | "number"
  | "select"
  | "checkbox"
  | "repo"; // review-bot allowlisted repo picker (falls back to text)

export interface AgentFieldOption {
  value: string;
  label: string;
}

export interface AgentField {
  key: string;
  label: string;
  type: AgentFieldType;
  required?: boolean;
  /** Minimum trimmed length for text/textarea/repo (default 1 when required). */
  minLength?: number;
  /** Minimum numeric value for number fields (default 1 when required). */
  min?: number;
  placeholder?: string;
  help?: string;
  options?: AgentFieldOption[];
  defaultValue?: string | boolean;
}

/** Minimum length for board task titles (create + edit). */
export const TASK_TITLE_MIN_LENGTH = 3;

export type AgentBuiltin =
  | "article_writer"
  | "researcher"
  | "pentester"
  | "pr_reviewer"
  | "mail";

/**
 * How a selected agent should be launched from Task Manager.
 * Specialists hit their dedicated APIs; everything else uses task runs.
 *
 * Keep required field keys and order in sync with server/internal/agentschema.
 */
export type AgentLaunchKind = "task_run" | AgentBuiltin | "images";

export interface AgentInputSchema {
  kind: AgentLaunchKind;
  /** Short blurb under the agent picker. */
  hint: string;
  fields: AgentField[];
  /**
   * Build a board-task title from the filled values. Specialists don't ask for
   * a separate title — the primary input becomes the title.
   */
  titleFrom?: (values: Record<string, string>) => string;
  /** Optional description derived from the form (stored on the board task). */
  descriptionFrom?: (values: Record<string, string>) => string | undefined;
}

const GENERIC: AgentInputSchema = {
  kind: "task_run",
  hint: "Describe the work. Title and description become the agent's prompt when you run.",
  fields: [
    {
      key: "title",
      label: "Title",
      type: "text",
      required: true,
      minLength: TASK_TITLE_MIN_LENGTH,
      placeholder: "e.g. Draft the launch announcement",
    },
    {
      key: "description",
      label: "Description / prompt",
      type: "textarea",
      placeholder: "What should the agent do?",
    },
  ],
  titleFrom: (v) => v.title?.trim() || "Untitled task",
  descriptionFrom: (v) => v.description?.trim() || undefined,
};

const SCHEMAS: Record<AgentBuiltin, AgentInputSchema> = {
  article_writer: {
    kind: "article_writer",
    hint: "Give a topic to research. The writer picks its own title from sources.",
    fields: [
      {
        key: "topic",
        label: "Topic",
        type: "text",
        required: true,
        minLength: 3,
        placeholder: "e.g. Edge AI inference in 2026",
      },
      {
        key: "context",
        label: "Context (optional)",
        type: "textarea",
        placeholder: "Audience, angle, points to cover or avoid",
      },
      {
        key: "model",
        label: "Model override (optional)",
        type: "text",
        placeholder: "agent default",
      },
      {
        key: "cover_style",
        label: "Cover accent",
        type: "select",
        help: "Auto varies the hue by topic. Pick one to pin every cover to it.",
        options: [
          { value: "", label: "Auto (varies by topic)" },
          { value: "teal and cyan", label: "Teal and cyan" },
          { value: "violet and indigo", label: "Violet and indigo" },
          { value: "amber and coral", label: "Amber and coral" },
          { value: "emerald and mint", label: "Emerald and mint" },
        ],
      },
      {
        key: "illustrations",
        label: "In-body illustrations",
        type: "select",
        defaultValue: "on",
        help: "Off keeps the cover but draws no pictures inside the article.",
        options: [
          { value: "on", label: "On" },
          { value: "off", label: "Off" },
        ],
      },
    ],
    titleFrom: (v) => `Write: ${v.topic?.trim() || "article"}`,
    descriptionFrom: (v) => {
      const parts = [`Topic: ${v.topic?.trim()}`];
      if (v.context?.trim()) parts.push(v.context.trim());
      return parts.join("\n\n");
    },
  },
  researcher: {
    kind: "researcher",
    hint: "Research a subject and return cited findings.",
    fields: [
      {
        key: "topic",
        label: "Topic",
        type: "text",
        required: true,
        minLength: 3,
        placeholder: "e.g. Kubernetes cost optimisation patterns",
      },
      {
        key: "context",
        label: "Context (optional)",
        type: "textarea",
        placeholder: "Angle, constraints, what to emphasise",
      },
    ],
    titleFrom: (v) => `Research: ${v.topic?.trim() || "topic"}`,
    descriptionFrom: (v) => {
      const parts = [`Topic: ${v.topic?.trim()}`];
      if (v.context?.trim()) parts.push(v.context.trim());
      return parts.join("\n\n");
    },
  },
  pentester: {
    kind: "pentester",
    hint: "Start a security scan against an authorised target.",
    fields: [
      {
        key: "target",
        label: "Target URL or path",
        type: "text",
        required: true,
        minLength: 3,
        placeholder: "https://int.example.com",
        help: "Live API or app URL you are authorised to test",
      },
      {
        key: "scan_mode",
        label: "Scan mode",
        type: "select",
        required: true,
        defaultValue: "quick",
        options: [
          { value: "quick", label: "Quick (5–15 min)" },
          { value: "standard", label: "Standard (30–60 min)" },
          { value: "deep", label: "Deep (1–2+ hours)" },
        ],
      },
      {
        key: "max_budget",
        label: "Max budget (USD cents, optional)",
        type: "number",
        min: 1,
        placeholder: "1000",
        help: "e.g. 1000 = $10",
      },
      {
        key: "instruction",
        label: "Engagement note (optional)",
        type: "textarea",
        placeholder: "Focus on /api; ignore marketing pages",
      },
    ],
    titleFrom: (v) => `Pentest: ${v.target?.trim() || "target"}`,
    descriptionFrom: (v) => {
      const parts = [`Target: ${v.target?.trim()}`, `Mode: ${v.scan_mode || "quick"}`];
      if (v.instruction?.trim()) parts.push(v.instruction.trim());
      return parts.join("\n");
    },
  },
  pr_reviewer: {
    kind: "pr_reviewer",
    hint: "Queue an AI review of a GitHub pull request.",
    fields: [
      {
        key: "repo",
        label: "Repository",
        type: "repo",
        required: true,
        minLength: 3,
        placeholder: "owner/name",
      },
      {
        key: "pr_number",
        label: "Pull request number",
        type: "number",
        required: true,
        min: 1,
        placeholder: "42",
      },
      {
        key: "dry_run",
        label: "Preview only — do not post comments on GitHub",
        type: "checkbox",
        defaultValue: false,
      },
    ],
    titleFrom: (v) =>
      `Review: ${v.repo?.trim() || "repo"}#${v.pr_number?.trim() || "?"}`,
    descriptionFrom: (v) =>
      `Review ${v.repo?.trim()}#${v.pr_number?.trim()}${
        v.dry_run === "true" ? " (preview only)" : ""
      }`,
  },
  mail: {
    kind: "mail",
    hint: "Saves watch rules and the knowledge playbook on the shared mailbox, then syncs Gmail. Connect Gmail on Mail Agent first if you have not. Nothing is sent until you Approve a draft.",
    fields: [
      {
        key: "senders",
        label: "Watch senders",
        type: "text",
        placeholder: "ops@example.com, support@client.com",
        help: "Comma-separated. Empty = all unread in the last 7 days.",
      },
      {
        key: "subject_prefixes",
        label: "Subject prefixes",
        type: "text",
        placeholder: "[support], [billing]",
      },
      {
        key: "labels",
        label: "Gmail labels",
        type: "text",
        placeholder: "INBOX, Support",
      },
      {
        key: "knowledge_urls",
        label: "Knowledge links",
        type: "textarea",
        placeholder: "https://example.com/pricing",
        help: "One http(s) URL per line. Matching mail is researched from these pages only. Empty = open web when the classifier asks.",
      },
      {
        key: "research_focus",
        label: "What to look for in those pages",
        type: "textarea",
        placeholder: "Prices, SLA, refund window…",
      },
      {
        key: "reply_instructions",
        label: "How the reply should read",
        type: "textarea",
        placeholder: "Tone, length, must-include, must-avoid",
      },
    ],
    titleFrom: (v) => {
      const focus = v.research_focus?.trim();
      if (focus) return `Mail: ${focus.slice(0, 80)}`;
      const urls = v.knowledge_urls?.trim();
      if (urls) return "Mail: research pinned pages and draft";
      return "Mail: sync inbox and draft";
    },
    descriptionFrom: (v) => {
      const parts: string[] = [];
      if (v.senders?.trim()) parts.push(`Senders: ${v.senders.trim()}`);
      if (v.knowledge_urls?.trim()) parts.push(v.knowledge_urls.trim());
      if (v.research_focus?.trim()) parts.push(`Look for: ${v.research_focus.trim()}`);
      if (v.reply_instructions?.trim()) {
        parts.push(`Reply style: ${v.reply_instructions.trim()}`);
      }
      return parts.length ? parts.join("\n\n") : undefined;
    },
  },
};

/** True when this schema launches via a specialist API (not generic task run). */
export function isSpecialistSchema(schema: AgentInputSchema): boolean {
  return schema.kind !== "task_run" && schema.kind !== "images";
}

/** Resolve the input schema for an agent (builtin specialist or generic). */
export function getAgentInputSchema(agent: Agent | null | undefined): AgentInputSchema {
  if (!agent) return GENERIC;
  const builtin = agent.metadata?.builtin;
  if (typeof builtin === "string" && builtin in SCHEMAS) {
    return SCHEMAS[builtin as AgentBuiltin];
  }
  return GENERIC;
}

/** Defaults for every field in a schema (stringified for form state). */
export function defaultValuesForSchema(
  schema: AgentInputSchema
): Record<string, string> {
  const out: Record<string, string> = {};
  for (const f of schema.fields) {
    if (f.type === "checkbox") {
      out[f.key] = f.defaultValue === true ? "true" : "false";
    } else if (f.defaultValue != null) {
      out[f.key] = String(f.defaultValue);
    } else {
      out[f.key] = "";
    }
  }
  return out;
}

/**
 * Validate every field against the schema. Returns per-field error messages
 * (empty object means the form is ready to submit / launch).
 */
export function validateSchemaValues(
  schema: AgentInputSchema,
  values: Record<string, string>
): Record<string, string> {
  const errors: Record<string, string> = {};
  for (const f of schema.fields) {
    const raw = (values[f.key] ?? "").trim();
    const err = validateField(f, raw);
    if (err) errors[f.key] = err;
  }
  return errors;
}

function validateField(f: AgentField, raw: string): string | null {
  if (f.type === "checkbox") return null;

  if (!raw) {
    if (f.required) return `${f.label} is required`;
    return null;
  }

  if (f.type === "number") {
    const n = Number(raw);
    if (!Number.isFinite(n) || !Number.isInteger(n)) {
      return `${f.label} must be a whole number`;
    }
    const min = f.min ?? (f.required ? 1 : undefined);
    if (min != null && n < min) {
      return `${f.label} must be at least ${min}`;
    }
    return null;
  }

  if (f.type === "select" && f.options) {
    if (!f.options.some((o) => o.value === raw)) {
      return `Choose a valid ${f.label.toLowerCase()}`;
    }
    return null;
  }

  if (f.type === "repo") {
    // owner/name — allow github.com URLs too; launch/API normalises.
    if (!raw.includes("/") || raw.startsWith("/") || raw.endsWith("/")) {
      return "Use owner/name (e.g. acme/api)";
    }
  }

  const minLen = f.minLength ?? (f.required ? 1 : undefined);
  if (minLen != null && raw.length < minLen) {
    return `${f.label} must be at least ${minLen} characters`;
  }
  return null;
}

/** Whether required fields are filled and type-valid. */
export function schemaValuesValid(
  schema: AgentInputSchema,
  values: Record<string, string>
): boolean {
  return Object.keys(validateSchemaValues(schema, values)).length === 0;
}

/** Board title used in edit mode. */
export function validateTaskTitle(title: string): string | null {
  const t = title.trim();
  if (!t) return "Title is required";
  if (t.length < TASK_TITLE_MIN_LENGTH) {
    return `Title must be at least ${TASK_TITLE_MIN_LENGTH} characters`;
  }
  return null;
}

/** Derive board title/description from schema + values. */
export function taskFieldsFromValues(
  schema: AgentInputSchema,
  values: Record<string, string>
): { title: string; description?: string } {
  return {
    title: schema.titleFrom?.(values) ?? values.title?.trim() ?? "Untitled task",
    description: schema.descriptionFrom?.(values),
  };
}

/**
 * Structured inputs for POST /tasks/{id}/run (generic agents).
 * Skips title/description which already form the prompt.
 */
export function runInputsFromValues(
  schema: AgentInputSchema,
  values: Record<string, string>
): Record<string, unknown> {
  if (schema.kind !== "task_run") {
    const out: Record<string, unknown> = {};
    for (const f of schema.fields) {
      const raw = values[f.key];
      if (raw == null || raw === "") continue;
      if (f.type === "checkbox") {
        out[f.key] = raw === "true";
      } else if (f.type === "number") {
        const n = Number(raw);
        if (!Number.isNaN(n)) out[f.key] = n;
      } else {
        out[f.key] = raw;
      }
    }
    return out;
  }
  // Generic: only pass extra keys beyond title/description
  const out: Record<string, unknown> = {};
  for (const [k, v] of Object.entries(values)) {
    if (k === "title" || k === "description") continue;
    if (v.trim()) out[k] = v;
  }
  return out;
}

/**
 * String-valued inputs for POST /agent-runs.
 *
 * The server's interview contract (server/internal/agentschema) is string-keyed
 * throughout — chat collects strings, and each runner parses what it needs — so
 * numbers and checkboxes are sent as their string form rather than as JSON
 * types. That keeps one representation of "the answers to the interview"
 * instead of one per surface.
 *
 * Generic agents are the exception worth naming: this form asks for a title and
 * a description, while the server's generic schema asks for a single prompt, so
 * the two are joined here. When the run belongs to a board task the server
 * rebuilds the prompt from the task itself; this value is what a run with no
 * task uses.
 */
export function agentRunInputs(
  schema: AgentInputSchema,
  values: Record<string, string>
): Record<string, string> {
  const out: Record<string, string> = {};

  if (schema.kind === "task_run") {
    const title = values.title?.trim() ?? "";
    const description = values.description?.trim() ?? "";
    const prompt = [title, description].filter(Boolean).join("\n\n");
    if (prompt) out.prompt = prompt;
    for (const [k, v] of Object.entries(values)) {
      if (k === "title" || k === "description") continue;
      if (v?.trim()) out[k] = v.trim();
    }
    return out;
  }

  for (const f of schema.fields) {
    const raw = values[f.key];
    if (raw == null || raw === "") continue;
    out[f.key] = f.type === "checkbox" ? String(raw === "true") : raw;
  }
  return out;
}
