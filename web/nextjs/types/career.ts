export type CareerStatus =
  | "evaluated"
  | "applied"
  | "responded"
  | "interview"
  | "offer"
  | "rejected"
  | "discarded"
  | "skip"
  | "hired";

export interface CareerIdentity {
  full_name?: string;
  email?: string;
  phone?: string;
  links?: string[];
}

export interface CareerTargets {
  titles?: string[];
  seniority?: string;
  industries?: string[];
  min_comp?: string;
}

export interface CareerWorkAuth {
  countries?: string[];
  needs_sponsorship?: boolean;
  authorized_already?: boolean;
}

export interface CareerLocation {
  cities?: string[];
  remote?: boolean;
  relocation?: boolean;
}

export interface CareerProfile {
  id: string;
  org_id: string;
  user_id: string;
  cv_markdown: string;
  identity: CareerIdentity;
  targets: CareerTargets;
  location: CareerLocation;
  work_auth: CareerWorkAuth;
  voice: string;
  house_rules: string;
  proof_points: string;
  narrative: string;
  created_at: string;
  updated_at: string;
}

export interface CareerScore {
  overall: number;
  dimensions?: Record<string, number>;
  recommend_apply: boolean;
  recommend_form_answers: boolean;
  recommendation?: string;
}

export interface CareerEvalBlocks {
  a?: string;
  b?: string;
  c?: string;
  d?: string;
  e?: string;
  f?: string;
  g?: string;
  h?: string;
  work_auth?: string;
}

export interface CareerEvaluation {
  id: string;
  company: string;
  role: string;
  listing_url: string;
  jd_text: string;
  blocks: CareerEvalBlocks;
  score: CareerScore;
  report_markdown: string;
  legitimacy_tier: string;
  hard_stop: boolean;
  hard_stop_reason: string;
  mode: string;
  application_id?: string | null;
  created_at: string;
}

export interface CareerApplication {
  id: string;
  company: string;
  role: string;
  listing_url: string;
  status: CareerStatus;
  score?: number | null;
  via: string;
  agency: string;
  created_at: string;
  updated_at: string;
}

export interface CareerPipelineItem {
  id: string;
  listing_url: string;
  company: string;
  title: string;
  source: string;
  status: string;
  liveness: string;
  created_at: string;
}

export interface CareerArtifact {
  id: string;
  kind: string;
  title: string;
  body_markdown: string;
  has_pdf: boolean;
  created_at: string;
}

export interface CareerEvaluateResult {
  evaluation?: CareerEvaluation;
  application?: CareerApplication;
  blacklist_hit?: { company: string; domain: string; reason: string };
  dead?: boolean;
  dead_reason?: string;
  artifacts?: CareerArtifact[];
}

export interface CareerDoctorReport {
  ok: boolean;
  warnings: string[];
  info: string[];
}

export interface CareerPatterns {
  applications: number;
  by_status: Record<string, number>;
  avg_score: number;
  skill_gaps?: string[];
}

export interface Paginated<T> {
  data: T[];
  total: number;
  page: number;
  per_page: number;
  total_pages: number;
}

export interface CareerPortal {
  id: string;
  board: string;
  slug: string;
  company: string;
  title_include: string[];
  title_exclude: string[];
  enabled: boolean;
}

export interface CareerBlacklistEntry {
  id: string;
  company: string;
  domain: string;
  reason: string;
}

export interface CareerIntakeProposal {
  summary: string;
  patch: {
    cv_markdown?: string;
    identity?: CareerIdentity;
  };
}

export interface CareerStory {
  id: string;
  title: string;
  situation: string;
  task: string;
  action: string;
  result: string;
  reflection: string;
  provenance: string;
  tags: string[];
}

export interface CareerFollowup {
  id: string;
  application_id: string;
  due_at: string;
  kind: string;
  draft: string;
  sent: boolean;
}

export interface CareerInterviewPrep {
  company: string;
  role: string;
  score_floor_met: boolean;
  stories: CareerStory[];
  prep_markdown: string;
  never_submit: boolean;
}

export interface CareerOfferPrep {
  company: string;
  role: string;
  prep_markdown: string;
  not_legal_advice: boolean;
}

export interface CareerSalaryGap {
  desired: string;
  advertised: string;
  actual: string;
  note: string;
  not_legal_advice: boolean;
}

export interface CareerContact {
  id: string;
  name: string;
  role: string;
  company: string;
  linkedin_draft: string;
}
