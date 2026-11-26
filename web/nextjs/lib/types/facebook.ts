/** drafting → draft → scheduled → publishing → published; failed rows can be redrafted. */
export type FacebookPostStatus =
  | "drafting"
  | "draft"
  | "scheduled"
  | "publishing"
  | "published"
  | "failed";

export interface FacebookPost {
  id: string;
  page_id: string;
  status: FacebookPostStatus;
  message: string;
  image_url?: string;
  /** What the creative should show; the picture is generated from it. */
  image_prompt?: string;
  category?: string;
  audience?: string;
  objective?: string;
  /** Suggested time on a draft; the publish time once scheduled. */
  scheduled_for?: string;
  error_message?: string;
  fb_post_id?: string;
  post_url?: string;
  published_at?: string;
  reactions: number;
  comments: number;
  shares: number;
  created_at: string;
  updated_at: string;
}

export interface FacebookPage {
  id: string;
  page_id: string;
  name: string;
  category: string;
  about: string;
  description: string;
  website: string;
  followers: number;
  /** Meta rejected the token; connecting again restores it. */
  needs_reauth: boolean;
  brief: string;
  voice: string;
  timezone: string;
  paused: boolean;
}

export interface FacebookPageCreation {
  supported: boolean;
  reason: string;
  steps: string[];
}

export interface FacebookStatus {
  /** False when the deployment has no Meta app configured. */
  configured: boolean;
  connected: boolean;
  name?: string;
  expires_at?: string;
  pages: FacebookPage[];
  page_creation: FacebookPageCreation;
}

export interface FacebookCategoryResult {
  category: string;
  posts: number;
  avg_engagement_per_post: number;
}

export interface FacebookAnalytics {
  page_id: string;
  posts: number;
  reactions: number;
  comments: number;
  shares: number;
  avg_engagement_per_post: number;
  followers: number;
  top_post?: FacebookPost;
  by_category: FacebookCategoryResult[];
}

export interface FacebookComment {
  id: string;
  from: string;
  message: string;
  created_time: string;
}

/** The editor's limit; Facebook accepts more but a post that long is not a good post. */
export const FACEBOOK_MESSAGE_MAX = 5000;

export const FACEBOOK_STATUS_LABEL: Record<FacebookPostStatus, string> = {
  drafting: "Drafting",
  draft: "Draft",
  scheduled: "Scheduled",
  publishing: "Publishing",
  published: "Published",
  failed: "Failed",
};

export function isFacebookPostActive(status: FacebookPostStatus): boolean {
  return status === "drafting" || status === "publishing";
}
