"use client";

import { useMemo } from "react";
import { useAvailableModels } from "@/lib/hooks/useLLMProviders";
import type { AvailableModel } from "@/lib/types/llm-provider";

/** The two fields an agent stores. Mirrors the API shape exactly. */
export interface ModelSelection {
  provider: string;
  model: string;
}

interface ModelPickerProps {
  value: ModelSelection;
  onChange: (value: ModelSelection) => void;
  id?: string;
  disabled?: boolean;
  /** Offer "Auto" when the server has auto-selection enabled. */
  includeAuto?: boolean;
  className?: string;
}

/** How a provider key is titled in the dropdown. */
const PROVIDER_LABELS: Record<string, string> = {
  ollama: "Ollama (local)",
  openai: "OpenAI",
  claude: "Claude",
};

/** Compact context-window label: 262144 -> "256k". */
function formatContext(tokens: number): string {
  if (!tokens) return "";
  if (tokens >= 1024) return `${Math.round(tokens / 1024)}k ctx`;
  return `${tokens} ctx`;
}

function describe(m: AvailableModel): string {
  const parts = [m.name];
  if (m.parameter_size) parts.push(m.parameter_size);
  const ctx = formatContext(m.context_tokens);
  if (ctx) parts.push(ctx);
  if (m.supports_tools) parts.push("tools");
  if (m.supports_vision) parts.push("vision");
  return parts.join(" · ");
}

/** One entry in the flattened option list. */
interface Option {
  label: string;
  selection: ModelSelection;
  group: string;
}

const SELECT_CLASS =
  "mt-1.5 flex h-9 w-full rounded-md border border-input bg-background px-3 text-sm " +
  "ring-offset-background focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring " +
  "disabled:cursor-not-allowed disabled:opacity-50";

/**
 * Grouped model dropdown, populated from what each provider can actually run.
 *
 * The `<option value>` is the index into a flattened option list rather than an
 * encoded "provider:model" string. The value never leaves this component, so
 * inventing a string format — and then having to split it carefully, because
 * Ollama model names contain colons — would be pure downside.
 */
export function ModelPicker({
  value,
  onChange,
  id,
  disabled,
  includeAuto = true,
  className,
}: ModelPickerProps) {
  const { data, isLoading, isError } = useAvailableModels();

  const options = useMemo<Option[]>(() => {
    const out: Option[] = [
      {
        label: "Platform default",
        selection: { provider: "", model: "" },
        group: "Default",
      },
    ];

    if (includeAuto && data?.auto.available) {
      out.push({
        label: data.auto.label,
        selection: { provider: "auto", model: "" },
        group: "Automatic",
      });
    }

    for (const p of data?.providers ?? []) {
      const group = PROVIDER_LABELS[p.provider] ?? p.provider;
      for (const m of p.models) {
        out.push({
          label: describe(m),
          selection: { provider: p.provider, model: m.name },
          group,
        });
      }
    }

    return out;
  }, [data, includeAuto]);

  // An agent may hold a model that is no longer installed, or free text typed
  // before this picker existed. Surface it rather than silently rewriting the
  // agent's configuration on first render.
  const selectedIndex = options.findIndex(
    (o) => o.selection.provider === value.provider && o.selection.model === value.model
  );
  const orphan =
    selectedIndex === -1 && (value.provider || value.model)
      ? `${value.provider || "?"} / ${value.model || "default"} (not available)`
      : null;

  const allOptions = orphan
    ? [{ label: orphan, selection: value, group: "Current" }, ...options]
    : options;
  const currentIndex = orphan ? 0 : Math.max(selectedIndex, 0);

  // Preserve declaration order while grouping, so "Default" and "Automatic"
  // stay at the top and providers follow in the order the API returned them.
  const groups = allOptions.reduce<{ name: string; items: { option: Option; index: number }[] }[]>(
    (acc, option, index) => {
      const last = acc[acc.length - 1];
      if (last && last.name === option.group) last.items.push({ option, index });
      else acc.push({ name: option.group, items: [{ option, index }] });
      return acc;
    },
    []
  );

  if (isLoading) {
    return (
      <select id={id} className={className ?? SELECT_CLASS} disabled>
        <option>Loading models…</option>
      </select>
    );
  }

  return (
    <>
      <select
        id={id}
        data-testid="model-picker"
        value={currentIndex}
        disabled={disabled}
        onChange={(e) => onChange(allOptions[Number(e.target.value)].selection)}
        className={className ?? SELECT_CLASS}
      >
        {groups.map((g) => (
          <optgroup key={g.name} label={g.name}>
            {g.items.map(({ option, index }) => (
              <option key={index} value={index}>
                {option.label}
              </option>
            ))}
          </optgroup>
        ))}
      </select>
      {isError && (
        <p className="mt-1 text-xs text-destructive">
          Could not load the model list. The platform default will be used.
        </p>
      )}
    </>
  );
}
