"use client";

import { useEffect, useState } from "react";

import { apiClient } from "@/lib/api/client";
import type { AgentField } from "@/lib/agents/input-schemas";
import { parseTags, TagInput } from "@/components/ui/TagInput";
import type { ReviewRepos } from "@/types/review";
import { cn } from "@/lib/utils/cn";

const inputCls =
  "flex h-10 w-full rounded-md border bg-background px-3 py-2 text-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50";

interface AgentInputFieldsProps {
  fields: AgentField[];
  values: Record<string, string>;
  onChange: (key: string, value: string) => void;
  /** Per-field validation messages from validateSchemaValues. */
  errors?: Record<string, string>;
  disabled?: boolean;
  /** Focus the first required field when true (create flow after agent pick). */
  autoFocusFirst?: boolean;
}

/**
 * Renders the dynamic fields for the selected agent's input schema.
 */
export function AgentInputFields({
  fields,
  values,
  onChange,
  errors,
  disabled,
  autoFocusFirst,
}: AgentInputFieldsProps) {
  const firstRequired = fields.find((f) => f.required)?.key;

  let lastGroup: string | undefined;
  return (
    <div className="space-y-4">
      {fields.map((field) => {
        const showGroup = Boolean(field.group && field.group !== lastGroup);
        lastGroup = field.group ?? lastGroup;
        return (
          <div key={field.key} className="space-y-2">
            {showGroup ? (
              <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                {field.group}
              </p>
            ) : null}
            <Field
              field={field}
              value={values[field.key] ?? ""}
              onChange={(v) => onChange(field.key, v)}
              error={errors?.[field.key]}
              disabled={disabled}
              autoFocus={Boolean(autoFocusFirst && field.key === firstRequired)}
            />
          </div>
        );
      })}
    </div>
  );
}

function Field({
  field,
  value,
  onChange,
  error,
  disabled,
  autoFocus,
}: {
  field: AgentField;
  value: string;
  onChange: (v: string) => void;
  error?: string;
  disabled?: boolean;
  autoFocus?: boolean;
}) {
  const borderCls = error ? "border-destructive" : "border-input";

  useEffect(() => {
    if (field.type === "checkbox" && value === "" && field.defaultValue === true) {
      onChange("true");
    }
  }, [field.type, field.defaultValue, value, onChange]);

  if (field.type === "checkbox") {
    const checked =
      value === "true" || (value === "" && field.defaultValue === true);
    return (
      <label className="flex cursor-pointer items-start gap-2 text-sm">
        <input
          type="checkbox"
          checked={checked}
          onChange={(e) => onChange(e.target.checked ? "true" : "false")}
          disabled={disabled}
          className="mt-0.5 h-4 w-4 rounded border-input"
        />
        <span>
          {field.label}
          {field.help && (
            <span className="mt-0.5 block text-xs text-muted-foreground">
              {field.help}
            </span>
          )}
        </span>
      </label>
    );
  }

  const minLen = field.minLength ?? (field.required ? 1 : undefined);
  const minNum = field.min ?? (field.type === "number" && field.required ? 1 : undefined);

  return (
    <div className="space-y-1.5">
      <label className="text-sm font-medium" htmlFor={`agent-field-${field.key}`}>
        {field.label}
        {field.required && <span className="text-destructive"> *</span>}
      </label>
      {field.type === "textarea" ? (
        <textarea
          id={`agent-field-${field.key}`}
          rows={3}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          placeholder={field.placeholder}
          disabled={disabled}
          autoFocus={autoFocus}
          required={field.required}
          minLength={minLen}
          aria-invalid={Boolean(error)}
          className={cn(
            "w-full resize-none rounded-md border bg-background px-3 py-2 text-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50",
            borderCls
          )}
        />
      ) : field.type === "select" && field.options ? (
        <select
          id={`agent-field-${field.key}`}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          disabled={disabled}
          autoFocus={autoFocus}
          required={field.required}
          aria-invalid={Boolean(error)}
          className={cn(inputCls, borderCls)}
        >
          {field.options.map((o) => (
            <option key={o.value} value={o.value}>
              {o.label}
            </option>
          ))}
        </select>
      ) : field.type === "repo" ? (
        <RepoField
          id={`agent-field-${field.key}`}
          value={value}
          onChange={onChange}
          disabled={disabled}
          autoFocus={autoFocus}
          placeholder={field.placeholder}
          required={field.required}
          minLength={minLen}
          error={Boolean(error)}
        />
      ) : field.type === "tags" ? (
        <TagInput
          id={`agent-field-${field.key}`}
          value={parseTags(value)}
          onChange={(tags) => onChange(tags.join(", "))}
          placeholder={field.placeholder}
          disabled={disabled}
          size={field.key === "senders" || field.key === "titles" ? "lg" : "md"}
          className={error ? "border-destructive" : undefined}
        />
      ) : (
        <input
          id={`agent-field-${field.key}`}
          type={field.type === "number" ? "number" : "text"}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          placeholder={field.placeholder}
          disabled={disabled}
          autoFocus={autoFocus}
          required={field.required}
          minLength={field.type === "number" ? undefined : minLen}
          min={minNum}
          aria-invalid={Boolean(error)}
          className={cn(inputCls, borderCls)}
        />
      )}
      {error ? (
        <p className="text-xs text-destructive" role="alert">
          {error}
        </p>
      ) : field.help ? (
        <p className="text-xs text-muted-foreground">{field.help}</p>
      ) : null}
    </div>
  );
}

function RepoField({
  id,
  value,
  onChange,
  disabled,
  autoFocus,
  placeholder,
  required,
  minLength,
  error,
}: {
  id: string;
  value: string;
  onChange: (v: string) => void;
  disabled?: boolean;
  autoFocus?: boolean;
  placeholder?: string;
  required?: boolean;
  minLength?: number;
  error?: boolean;
}) {
  const [allowed, setAllowed] = useState<string[] | null>(null);
  const borderCls = error ? "border-destructive" : "border-input";

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const { data } = await apiClient.get<ReviewRepos>("/review-runs/repos");
        if (cancelled) return;
        const list = data.allowed ?? [];
        setAllowed(list);
        // Also replace a hydrated value that's no longer allowed: leaving it
        // rendered the <select> blank while the stale string still validated,
        // so "Run now" would launch against a repo the UI never displayed.
        if (list.length && (!value || !list.includes(value))) {
          onChange(list[0]);
        }
      } catch {
        if (!cancelled) setAllowed([]);
      }
    })();
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  if (allowed && allowed.length > 0) {
    return (
      <select
        id={id}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        disabled={disabled}
        autoFocus={autoFocus}
        required={required}
        aria-invalid={error}
        className={cn(inputCls, borderCls)}
      >
        {allowed.map((slug) => (
          <option key={slug} value={slug}>
            {slug}
          </option>
        ))}
      </select>
    );
  }

  return (
    <input
      id={id}
      type="text"
      value={value}
      onChange={(e) => onChange(e.target.value)}
      placeholder={placeholder ?? "owner/name"}
      disabled={disabled}
      autoFocus={autoFocus}
      required={required}
      minLength={minLength}
      aria-invalid={error}
      className={cn(inputCls, borderCls)}
    />
  );
}
