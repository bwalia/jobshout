import type { Priority, ProjectStatus, TaskStatus } from "./common";

export interface Project {
  id: string;
  org_id: string;
  name: string;
  description: string | null;
  status: ProjectStatus;
  priority: Priority;
  owner_id: string | null;
  due_date: string | null;
  created_at: string;
  updated_at: string;
}

export interface CreateProjectRequest {
  name: string;
  description?: string;
  priority?: Priority;
  due_date?: string;
}

export interface UpdateProjectRequest {
  name?: string;
  description?: string;
  status?: ProjectStatus;
  priority?: Priority;
  due_date?: string | null;
}

export interface Task {
  id: string;
  project_id: string;
  parent_id: string | null;
  title: string;
  description: string | null;
  status: TaskStatus;
  priority: Priority;
  assigned_agent_id: string | null;
  assigned_user_id: string | null;
  story_points: number | null;
  due_date: string | null;
  position: number;
  created_by: string | null;
  created_at: string;
  updated_at: string;
  labels?: TaskLabel[];
  subtask_count?: number;
}

export interface TaskLabel {
  label: string;
  color: string;
}

export interface CreateTaskRequest {
  project_id: string;
  title: string;
  description?: string;
  priority?: Priority;
  assigned_agent_id?: string;
  assigned_user_id?: string;
  story_points?: number;
  due_date?: string;
  parent_id?: string;
}

export interface UpdateTaskRequest {
  title?: string;
  description?: string;
  status?: TaskStatus;
  priority?: Priority;
  assigned_agent_id?: string | null;
  assigned_user_id?: string | null;
  story_points?: number | null;
  due_date?: string | null;
  position?: number;
}

export interface TaskComment {
  id: string;
  task_id: string;
  author_id: string | null;
  agent_id: string | null;
  body: string;
  created_at: string;
}

export interface Organization {
  id: string;
  name: string;
  slug: string;
  owner_id: string | null;
  created_at: string;
  updated_at: string;
}
