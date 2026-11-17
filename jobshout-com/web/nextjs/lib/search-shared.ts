/** Client-safe search kinds, labels, and board paths (no server fetches). */

export type SearchKind = "job" | "agent" | "talent" | "group" | "insight";

export type SearchHit = {
  kind: SearchKind;
  id: string;
  title: string;
  subtitle: string;
  href: string;
};

export type UnifiedSearchResponse = {
  query: string;
  results: SearchHit[];
};

const KIND_LABEL: Record<SearchKind, string> = {
  job: "Job",
  agent: "Agent",
  talent: "Talent",
  group: "Group",
  insight: "Insight",
};

export function searchKindLabel(kind: SearchKind): string {
  return KIND_LABEL[kind];
}

/** Board path when the user submits the scoped hero form (not a single hit). */
export function boardPathForKind(kind: SearchKind): string {
  switch (kind) {
    case "job":
      return "/jobs";
    case "agent":
      return "/agents";
    case "talent":
      return "/members";
    case "group":
      return "/groups";
    case "insight":
      return "/insights";
  }
}
