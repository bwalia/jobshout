export interface PaginatedResponse<T> {
  data: T[];
  total: number;
  page: number;
  per_page: number;
  total_pages: number;
}

export interface PaginationParams {
  page?: number;
  per_page?: number;
}

export type AgentStatus = "idle" | "active" | "paused" | "offline";

export type ProjectStatus = "active" | "paused" | "completed" | "archived";

export type TaskStatus =
  | "backlog"
  | "todo"
  | "in_progress"
  | "review"
  | "done";

export type Priority = "low" | "medium" | "high" | "critical";

export type UserRole = "admin" | "member";
