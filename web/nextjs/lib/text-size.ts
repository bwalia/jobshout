export type TextSize = "standard" | "large" | "largest";

export const TEXT_SIZES: { value: TextSize; label: string }[] = [
  { value: "standard", label: "Standard" },
  { value: "large", label: "Large" },
  { value: "largest", label: "Largest" },
];

// Read before first paint by the inline script in app/layout.tsx.
const KEY = "jobshout-text-size";

export function readTextSize(): TextSize {
  const v = document.documentElement.dataset.textSize;
  return v === "large" || v === "largest" ? v : "standard";
}

export function applyTextSize(size: TextSize) {
  const root = document.documentElement;
  if (size === "standard") delete root.dataset.textSize;
  else root.dataset.textSize = size;
  try {
    localStorage.setItem(KEY, size);
  } catch {
    // Private mode: the size still applies for this visit.
  }
}
