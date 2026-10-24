export type MarketingEmailRunStatus =
  | "writing"
  | "publishing"
  | "published"
  | "draft"
  | "failed"
  | "archived";

export interface MarketingEmailBrief {
  brief: string;
  audience?: string;
  goal?: string;
  primary_cta_label?: string;
  primary_cta_url?: string;
  products?: string;
  tone?: string;
  destination?: string;
  publish?: string;
  model?: string;
}

export interface MarketingEmailRun {
  id: string;
  org_id: string;
  task_id?: string;
  agent_id?: string;
  brief: MarketingEmailBrief;
  subject: string;
  preheader: string;
  title: string;
  slug: string;
  status: MarketingEmailRunStatus;
  stage: string;
  message: string;
  error?: string;
  content_html?: string;
  hero_image_url?: string;
  page_uuid?: string;
  page_status?: string;
  browser_url?: string;
  destination?: string;
  model?: string;
  publish_mode?: string;
  created_at: string;
  started_at?: string;
  finished_at?: string;
  updated_at: string;
}

export function isMarketingEmailRunActive(status: MarketingEmailRunStatus): boolean {
  return status === "writing" || status === "publishing";
}
