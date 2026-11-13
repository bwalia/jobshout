import { API_BASE, failure, headers, type Viewer } from "@/lib/insights";

export interface DashboardAction {
  id: string;
  title: string;
  reason: string;
  href: string;
  kind: string;
}

export interface DashboardOpportunity {
  id: string;
  kind: string;
  title: string;
  summary: string;
  href: string;
  score: number;
  reasons: string[];
}

export interface DashboardPipelineItem {
  id: string;
  kind: string;
  title: string;
  stage: string;
  next_step: string;
  href: string;
}

export interface Dashboard {
  today: DashboardAction[];
  opportunities: DashboardOpportunity[];
  pipeline: DashboardPipelineItem[];
  stats: {
    profile_views_7d: number;
    search_appearances_7d: number;
    follower_growth_7d: number;
    invitation_rate: number;
    proposal_win_rate: number;
    profile_strength: number;
    profile_fixes: string[];
  };
  credits: { balance: number; expiring_soon: number; plan: string };
  earnings_reported: number;
  unread_messages: number;
  unread_notifications: number;
  approval_inbox: Array<{ id: string; kind: string; title: string; preview: string; status: string }>;
  weekly_goals: DashboardAction[];
  roles: string[];
  widgets: string[];
}

export async function getDashboard(viewer: Viewer): Promise<Dashboard | null> {
  const res = await fetch(`${API_BASE}/api/v1/dashboard`, {
    headers: headers(viewer),
    cache: "no-store",
  });
  if (res.status === 404) return null;
  if (!res.ok) throw await failure(res, "Failed to load dashboard");
  return (await res.json()) as Dashboard;
}
