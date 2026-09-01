/**
 * Unified library items produced by agents. New kinds are added here so the
 * Artifacts panel can list them without growing a second navigation tree.
 */
export const ARTIFACT_KINDS = [
  { id: "article", label: "Articles", singular: "Article" },
  { id: "image", label: "Images", singular: "Image" },
] as const;

export type ArtifactKind = (typeof ARTIFACT_KINDS)[number]["id"];

export type ArtifactFilter = "all" | ArtifactKind;

export interface ArtifactItem {
  id: string;
  kind: ArtifactKind;
  title: string;
  subtitle?: string;
  href?: string;
  createdAt: string;
  status?: string;
  meta?: string;
  imageUrl?: string;
}
