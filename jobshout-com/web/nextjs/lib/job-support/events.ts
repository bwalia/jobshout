/** SSE events for POST /api/job-support/chat */

export type JobSupportEvent =
  | { type: "session"; session_id: string }
  | { type: "session_state"; state: Record<string, unknown> }
  | { type: "text"; text: string }
  | { type: "matches"; items: MatchItem[] }
  | { type: "status"; message: string }
  | { type: "done"; role?: "recruiter" | "candidate" | null }
  | { type: "error"; message: string; code?: string };

export type MatchItem = {
  id: string;
  title: string;
  score: number;
  summary: string;
  href?: string;
};

export function encodeSse(event: JobSupportEvent): string {
  return `data: ${JSON.stringify(event)}\n\n`;
}
