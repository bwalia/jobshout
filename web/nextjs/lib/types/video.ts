export type VideoRunStatus = "scripting" | "queued" | "rendering" | "done" | "failed" | "cancelled";

export interface VideoShot {
  narration: string;
  visual: string;
}

export interface VideoBrief {
  seconds: number;
  format: string;
  voice: string;
  style?: string;
  motion: boolean;
  captions: boolean;
  notes?: string;
}

/**
 * One Video Producer launch. `stage` and `message` are what the agent is doing
 * right now: writing the script, waiting for the workstation, then the
 * workstation's own steps ("motion" · "Animating shot 4 of 10").
 */
export interface VideoRun {
  id: string;
  org_id: string;
  task_id?: string;
  agent_id?: string;
  topic: string;
  brief: VideoBrief;
  title: string;
  shots: VideoShot[];
  status: VideoRunStatus;
  stage: string;
  progress: number;
  message: string;
  error?: string;
  notes: string[];
  video_url?: string;
  duration_seconds: number;
  motion_model?: string;
  /** Expected workstation minutes for the render. */
  estimate_minutes: number;
  created_at: string;
  started_at?: string;
  finished_at?: string;
  updated_at: string;
}

export function isVideoRunActive(status: VideoRunStatus): boolean {
  return status === "scripting" || status === "queued" || status === "rendering";
}
