"use client";

import { useEffect, useState } from "react";

import { apiClient } from "@/lib/api/client";
import type { AgentField } from "@/lib/agents/input-schemas";
import type { ReviewRepos } from "@/types/review";

const inputCls =
  "flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50";

interface AgentInputFieldsProps {
  fields: AgentField[];
  values: Record<string, string>;
  onChange: (key: string, value: string) => void;
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
  disabled,
  autoFocusFirst,
}: AgentInputFieldsProps) {
  const firstRequired = fields.find((f) => f.required)?.key;

  return (
    <div className="space-y-4">
      {fields.map((field) => (
        <Field
          key={field.key}
          field={field}
          value={values[field.key] ?? ""}
          onChange={(v) => onChange(field.key, v)}
          disabled={disabled}
          autoFocus={Boolean(autoFocusFirst && field.key === firstRequired)}
        />
      ))}
    </div>
  );
}

function Field({
  field,
  value,
  onChange,
  disabled,
  autoFocus,
}: {
  field: AgentField;
  value: string;
  onChange: (v: string) => void;
  disabled?: boolean;
  autoFocus?: boolean;
}) {
  if (field.type === "checkbox") {
    return (
      <label className="flex cursor-pointer items-start gap-2 text-sm">
        <input
          type="checkbox"
          checked={value === "true"}
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

  return (
    <div className="space-y-1.5">
      <label className="text-sm font-medium">
        {field.label}
        {field.required && <span className="text-destructive"> *</span>}
      </label>
      {field.type === "textarea" ? (
        <textarea
          rows={3}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          placeholder={field.placeholder}
          disabled={disabled}
          autoFocus={autoFocus}
          className="w-full resize-none rounded-md border border-input bg-background px-3 py-2 text-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50"
        />
      ) : field.type === "select" && field.options ? (
        <select
          value={value}
          onChange={(e) => onChange(e.target.value)}
          disabled={disabled}
          autoFocus={autoFocus}
          className={inputCls}
        >
          {field.options.map((o) => (
            <option key={o.value} value={o.value}>
              {o.label}
            </option>
          ))}
        </select>
      ) : field.type === "repo" ? (
        <RepoField
          value={value}
          onChange={onChange}
          disabled={disabled}
          autoFocus={autoFocus}
          placeholder={field.placeholder}
        />
      ) : (
        <input
          type={field.type === "number" ? "number" : "text"}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          placeholder={field.placeholder}
          disabled={disabled}
          autoFocus={autoFocus}
          min={field.type === "number" ? 1 : undefined}
          className={inputCls}
        />
      )}
      {field.help && (
        <p className="text-xs text-muted-foreground">{field.help}</p>
      )}
    </div>
  );
}

function RepoField({
  value,
  onChange,
  disabled,
  autoFocus,
  placeholder,
}: {
  value: string;
  onChange: (v: string) => void;
  disabled?: boolean;
  autoFocus?: boolean;
  placeholder?: string;
}) {
  const [allowed, setAllowed] = useState<string[] | null>(null);

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const { data } = await apiClient.get<ReviewRepos>("/review-runs/repos");
        if (cancelled) return;
        const list = data.allowed ?? [];
        setAllowed(list);
        if (list.length && !value) onChange(list[0]);
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
        value={value}
        onChange={(e) => onChange(e.target.value)}
        disabled={disabled}
        autoFocus={autoFocus}
        className={inputCls}
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
      type="text"
      value={value}
      onChange={(e) => onChange(e.target.value)}
      placeholder={placeholder ?? "owner/name"}
      disabled={disabled}
      autoFocus={autoFocus}
      className={inputCls}
    />
  );
}
