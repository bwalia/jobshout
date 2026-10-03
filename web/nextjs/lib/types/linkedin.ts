/** Who a draft is written for. */
export type LinkedInVariant = "technical" | "business";

/** drafting → draft → posting → posted; failed drafts can be redrafted. */
export type LinkedInPostStatus = "drafting" | "draft" | "posting" | "posted" | "failed";

export interface LinkedInPost {
  id: string;
  org_id: string;
  article_id: string;
  article_title: string;
  task_id?: string;
  variant: LinkedInVariant;
  status: LinkedInPostStatus;
  commentary: string;
  /** The article's public URL, attached as a link card. Empty: text-only post. */
  link_url: string;
  notes?: string;
  error_message?: string;
  post_urn?: string;
  post_url?: string;
  posted_at?: string;
  created_at: string;
  updated_at: string;
}

export interface LinkedInStatus {
  /** False when the deployment has no LinkedIn app configured. */
  configured: boolean;
  connected: boolean;
  /** LinkedIn gives members a 60-day token with no refresh. */
  expired: boolean;
  name?: string;
  expires_at?: string;
  days_left: number;
  auto_draft: boolean;
}

/** LinkedIn's limit on a post's text. */
export const LINKEDIN_COMMENTARY_MAX = 3000;

export const LINKEDIN_VARIANT_LABEL: Record<LinkedInVariant, string> = {
  technical: "Technical",
  business: "Business",
};

export function isLinkedInPostActive(status: LinkedInPostStatus): boolean {
  return status === "drafting" || status === "posting";
}
