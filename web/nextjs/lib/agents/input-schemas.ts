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
  /** Optional section heading shown above this field in Task Manager. */
  group?: string;
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
  | "mail"
  | "images";

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
        defaultValue: true,
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
    hint: "Saves who to watch and how to answer, then syncs Gmail. Connect Gmail on Mail Agent first if you have not. Nothing is sent until you Approve a draft. Links inside incoming mail are researched automatically.",
    fields: [
      {
        key: "senders",
        label: "Watch senders",
        type: "text",
        group: "Who to watch",
        placeholder: "ops@example.com, support@client.com",
        help: "Comma-separated. Empty = all unread mail from the last 7 days.",
      },
      {
        key: "subject_prefixes",
        label: "Subject prefixes",
        type: "text",
        group: "Who to watch",
        placeholder: "[support], [billing]",
      },
      {
        key: "labels",
        label: "Gmail labels",
        type: "text",
        group: "Who to watch",
        placeholder: "INBOX, Support",
      },
      {
        key: "knowledge_notes",
        label: "What the agent should know",
        type: "textarea",
        group: "How to answer",
        placeholder:
          "Mac Studio M5 Max: $2,499\nMac Studio M5 Ultra: $5,499\nRefunds within 30 days, shipping 3–5 working days…",
        help: "Prices, products, policies — plain text or markdown. Replies quote only what is written here.",
      },
      {
        key: "knowledge_urls",
        label: "Knowledge links (optional)",
        type: "textarea",
        group: "How to answer",
        placeholder: "https://example.com/pricing",
        help: "Optional pages to research on top of your notes (one URL per line). Incoming mail links are researched too.",
      },
      {
        key: "research_focus",
        label: "What to look for",
        type: "textarea",
        group: "How to answer",
        placeholder: "Prices, SLA, refund window…",
      },
      {
        key: "reply_instructions",
        label: "How the reply should read",
        type: "textarea",
        group: "How to answer",
        placeholder: "Tone, length, must-include, must-avoid",
      },
    ],
    titleFrom: (v) => {
      const focus = v.research_focus?.trim();
      if (focus) return `Mail: ${focus.slice(0, 80)}`;
      if (v.knowledge_notes?.trim()) return "Mail: draft from operator knowledge";
      const urls = v.knowledge_urls?.trim();
      if (urls) return "Mail: research pinned pages and draft";
      return "Mail: sync inbox and draft";
    },
    descriptionFrom: (v) => {
      const parts: string[] = [];
      if (v.senders?.trim()) parts.push(`Senders: ${v.senders.trim()}`);
      if (v.knowledge_notes?.trim()) {
        parts.push(`Knowledge: ${v.knowledge_notes.trim().slice(0, 200)}`);
      }
      if (v.knowledge_urls?.trim()) parts.push(v.knowledge_urls.trim());
      if (v.research_focus?.trim()) parts.push(`Look for: ${v.research_focus.trim()}`);
      if (v.reply_instructions?.trim()) {
        parts.push(`Reply style: ${v.reply_instructions.trim()}`);
      }
      return parts.length ? parts.join("\n\n") : undefined;
    },
  },
  images: {
    kind: "images",
    hint: "Generate one image from a prompt. The board task stores the result.",
    fields: [
      {
        key: "prompt",
        label: "Image prompt",
        type: "textarea",
        required: true,
        minLength: 3,
        placeholder: "A dark editorial cover of a harbour at night…",
      },
    ],
    titleFrom: (v) => {
      const p = v.prompt?.trim() || "image";
      return `Image: ${p.slice(0, 80)}`;
    },
    descriptionFrom: (v) => v.prompt?.trim() || undefined,
  },
};

/** True when this schema launches via the specialist launcher (not generic task run). */
export function isSpecialistSchema(schema: AgentInputSchema): boolean {
  return schema.kind !== "task_run";
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
