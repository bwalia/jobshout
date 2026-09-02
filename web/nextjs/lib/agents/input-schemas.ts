import type { Agent } from "@/lib/types/agent";

/** Field types the task-manager forms can render. */
export type AgentFieldType =
  | "text"
  | "textarea"
  | "number"
  | "select"
  | "checkbox"
  | "repo";

export interface AgentFieldOption {
  value: string;
  label: string;
}

export interface AgentField {
  key: string;
  label: string;
  type: AgentFieldType;
  required?: boolean;
  minLength?: number;
  min?: number;
  placeholder?: string;
  help?: string;
  group?: string;
  options?: AgentFieldOption[];
  defaultValue?: string | boolean;
}

export const TASK_TITLE_MIN_LENGTH = 3;

export type AgentBuiltin =
  | "article_writer"
  | "researcher"
  | "pentester"
  | "pr_reviewer"
  | "mail"
  | "images"
  | "career_ops"
  | string;

export type AgentLaunchKind = "task_run" | AgentBuiltin;

export interface TitleRule {
  if_key?: string;
  prefix?: string;
  from_key?: string;
  from_keys?: string[];
  format?: string;
  literal?: string;
  truncate?: number;
  fallback?: string;
  suffix_if?: string;
  suffix?: string;
}

export interface DescRule {
  prefix?: string;
  key?: string;
  truncate?: number;
  literal?: string;
  format?: string;
  suffix_if?: string;
  suffix?: string;
}

export interface RequireGroup {
  keys: string[];
  slot?: string;
  question?: string;
}

/** One builtin as GET /api/v1/agent-schemas returns it. */
export interface WireSchema {
  builtin: string;
  specialist_tool?: string;
  hint?: string;
  label?: string;
  icon?: string;
  tab_slug?: string;
  stay_on_tab?: boolean;
  prefill?: string;
  fields: {
    key: string;
    label: string;
    question?: string;
    type?: string;
    required: boolean;
    min_length?: number;
    min?: number;
    default?: string;
    placeholder?: string;
    help?: string;
    group?: string;
    options?: { label: string; value: string }[];
  }[];
  title_rules?: TitleRule[];
  desc_rules?: DescRule[];
  require_any?: RequireGroup[];
}

export interface AgentInputSchema {
  kind: AgentLaunchKind;
  hint: string;
  prefill?: string;
  fields: AgentField[];
  titleFrom?: (values: Record<string, string>) => string;
  descriptionFrom?: (values: Record<string, string>) => string | undefined;
  requireAny?: RequireGroup[];
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

function expand(format: string, v: Record<string, string>): string {
  return format.replace(/\{([a-z_]+)\}/g, (_, key: string) =>
    (v[key] ?? "").trim()
  );
}

function applyTitleRules(
  rules: TitleRule[] | undefined,
  v: Record<string, string>
): string {
  if (!rules?.length) {
    return (v.title?.trim() || v.prompt?.trim() || "Untitled task");
  }
  for (const r of rules) {
    if (r.if_key && !(v[r.if_key] ?? "").trim()) continue;
    if (r.literal) return r.literal;
    let part = "";
    if (r.from_key) part = (v[r.from_key] ?? "").trim();
    if (!part && r.from_keys) {
      for (const k of r.from_keys) {
        const s = (v[k] ?? "").trim();
        if (s) {
          part = s;
          break;
        }
      }
    }
    if (!part && r.fallback) part = r.fallback;
    if (r.truncate && part.length > r.truncate) part = part.slice(0, r.truncate);
    let out = r.prefix ?? "";
    out += r.format ? expand(r.format, v) : part;
    if (
      r.suffix_if &&
      (v[r.suffix_if] === "true" ||
        (v[r.suffix_if] === "" && r.suffix_if === "dry_run"))
    ) {
      out += r.suffix ?? "";
    }
    if (out.trim() !== (r.prefix ?? "") || part || r.format) return out;
  }
  return "Untitled task";
}

function applyDescRules(
  rules: DescRule[] | undefined,
  v: Record<string, string>
): string {
  if (!rules?.length) return "";
  const parts: string[] = [];
  for (const r of rules) {
    if (r.literal) {
      parts.push(r.literal);
      continue;
    }
    if (r.format) {
      let line = expand(r.format, v);
      if (
        r.suffix_if &&
        (v[r.suffix_if] === "true" ||
          (v[r.suffix_if] === "" && r.suffix_if === "dry_run"))
      ) {
        line += r.suffix ?? "";
      }
      parts.push(line);
      continue;
    }
    let raw = (v[r.key ?? ""] ?? "").trim();
    if (!raw) continue;
    if (r.truncate && raw.length > r.truncate) raw = raw.slice(0, r.truncate);
    parts.push(r.prefix ? r.prefix + raw : raw);
  }
  return parts.join("\n\n");
}

export function schemaFromWire(w: WireSchema): AgentInputSchema {
  return {
    kind: w.builtin,
    hint: w.hint ?? "",
    prefill: w.prefill,
    requireAny: w.require_any,
    fields: w.fields.map((f) => ({
      key: f.key,
      label: f.label,
      type: (f.type as AgentFieldType) || "text",
      required: f.required,
      minLength: f.min_length,
      min: f.min,
      placeholder: f.placeholder,
      help: f.help,
      group: f.group,
      options: f.options?.map((o) => ({ label: o.label, value: o.value })),
      defaultValue:
        f.type === "checkbox" ? f.default === "true" : f.default || undefined,
    })),
    titleFrom: (v) => applyTitleRules(w.title_rules, v),
    descriptionFrom: (v) => applyDescRules(w.desc_rules, v) || undefined,
  };
}

export function isSpecialistSchema(schema: AgentInputSchema): boolean {
  return schema.kind !== "task_run";
}

/**
 * Resolve the input schema for an agent.
 *
 * All specialists are wired this way: schema from GET /api/v1/agent-schemas.
 * A new agent does not need a TypeScript SCHEMAS map — register it.
 */
export function getAgentInputSchema(
  agent: Agent | null | undefined,
  catalog: WireSchema[] = []
): AgentInputSchema {
  if (!agent) return GENERIC;
  const builtin = agent.metadata?.builtin;
  if (typeof builtin === "string") {
    const wire = catalog.find((s) => s.builtin === builtin);
    if (wire) return schemaFromWire(wire);
  }
  return GENERIC;
}

export function defaultValuesForSchema(
  schema: AgentInputSchema
): Record<string, string> {
  const out: Record<string, string> = {};
  for (const f of schema.fields) {
    if (f.type === "checkbox") {
      out[f.key] = f.defaultValue === true || f.defaultValue === "true" ? "true" : "false";
    } else if (f.defaultValue != null) {
      out[f.key] = String(f.defaultValue);
    } else {
      out[f.key] = "";
    }
  }
  return out;
}

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
  for (const g of schema.requireAny ?? []) {
    const anyFilled = g.keys.some((k) => (values[k] ?? "").trim());
    if (!anyFilled) {
      const slot = g.slot || g.keys[0];
      errors[slot] = g.question || "Paste a job URL or a job description";
    }
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

export function schemaValuesValid(
  schema: AgentInputSchema,
  values: Record<string, string>
): boolean {
  return Object.keys(validateSchemaValues(schema, values)).length === 0;
}

export function validateTaskTitle(title: string): string | null {
  const t = title.trim();
  if (!t) return "Title is required";
  if (t.length < TASK_TITLE_MIN_LENGTH) {
    return `Title must be at least ${TASK_TITLE_MIN_LENGTH} characters`;
  }
  return null;
}

export function taskFieldsFromValues(
  schema: AgentInputSchema,
  values: Record<string, string>
): { title: string; description?: string } {
  return {
    title: schema.titleFrom?.(values) ?? values.title?.trim() ?? "Untitled task",
    description: schema.descriptionFrom?.(values),
  };
}

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
  const out: Record<string, unknown> = {};
  for (const [k, v] of Object.entries(values)) {
    if (k === "title" || k === "description") continue;
    if (v.trim()) out[k] = v;
  }
  return out;
}
