"use client";

import { useMemo } from "react";
import { useImageModels } from "@/lib/hooks/useImages";
import type { ImageModel } from "@/lib/types/image";

/** The two fields a choice consists of. Mirrors the API shape exactly. */
export interface ImageModelSelection {
  provider: string;
  model: string;
}

interface ImageModelPickerProps {
  value: ImageModelSelection;
  onChange: (value: ImageModelSelection) => void;
  id?: string;
  disabled?: boolean;
  className?: string;
}

/** How a provider key is titled in the dropdown. */
const PROVIDER_LABELS: Record<string, string> = {
  mflux: "Local (workstation)",
  openai: "OpenAI",
};

function providerLabel(provider: string): string {
  return PROVIDER_LABELS[provider] ?? provider;
}

/**
 * How a model reads in the list.
 *
 * "not downloaded" is the label that earns its place: mflux knows about thirty
 * models and has the weights for one. Selecting any other starts a
 * multi-gigabyte download that looks, from the UI, exactly like a generation
 * that never finishes — so the difference is stated rather than implied.
 */
function describe(m: ImageModel): string {
  const notes: string[] = [];
  if (m.fast) notes.push("fast");
  if (!m.available) notes.push("not downloaded");
  return notes.length ? `${m.name} — ${notes.join(", ")}` : m.name;
}

/**
 * Chooses which image model draws a picture.
 *
 * Deliberately shaped like the per-agent LLM ModelPicker: an operator who has
 * learned one control should not have to learn a second.
 */
export function ImageModelPicker({
  value,
  onChange,
  id = "image-model",
  disabled = false,
  className = "",
}: ImageModelPickerProps) {
  const { data, isLoading } = useImageModels();

  const grouped = useMemo(() => {
    const models = data?.models ?? [];
    const byProvider = new Map<string, ImageModel[]>();
    for (const m of models) {
      const list = byProvider.get(m.provider) ?? [];
      list.push(m);
      byProvider.set(m.provider, list);
    }
    return Array.from(byProvider.entries());
  }, [data]);

  /**
   * A stored choice naming a model the server no longer offers is surfaced at
   * the top rather than silently replaced. Overwriting it would discard a
   * deliberate decision because a workstation happened to be asleep when the
   * page loaded.
   */
  const orphaned = useMemo(() => {
    if (!value.model || !data?.models) return null;
    const known = data.models.some(
      (m) => m.name === value.model && m.provider === value.provider,
    );
    return known ? null : value.model;
  }, [value, data]);

  const serialize = (provider: string, model: string) => `${provider}::${model}`;

  const handleChange = (raw: string) => {
    if (raw === "") {
      onChange({ provider: "", model: "" });
      return;
    }
    const [provider, ...rest] = raw.split("::");
    onChange({ provider, model: rest.join("::") });
  };

  if (!isLoading && data && !data.enabled) {
    return (
      <select
        id={id}
        disabled
        className={`w-full rounded-md border border-border bg-muted px-3 py-2 text-sm text-muted-foreground ${className}`}
      >
        <option>Image generation is not configured on this server</option>
      </select>
    );
  }

  return (
    <select
      id={id}
      value={value.model ? serialize(value.provider, value.model) : ""}
      onChange={(e) => handleChange(e.target.value)}
      disabled={disabled || isLoading}
      className={`w-full rounded-md border border-border bg-background px-3 py-2 text-sm ${className}`}
    >
      <option value="">
        {isLoading
          ? "Loading models…"
          : `Platform default${data?.default_provider ? ` (${providerLabel(data.default_provider)})` : ""}`}
      </option>

      {orphaned && (
        <option value={serialize(value.provider, orphaned)}>
          {orphaned} (not available)
        </option>
      )}

      {grouped.map(([provider, models]) => (
        <optgroup key={provider} label={providerLabel(provider)}>
          {models.map((m) => (
            <option key={`${provider}::${m.name}`} value={serialize(provider, m.name)}>
              {describe(m)}
            </option>
          ))}
        </optgroup>
      ))}
    </select>
  );
}
