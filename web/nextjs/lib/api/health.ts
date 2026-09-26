const API_BASE_URL = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";

export interface HealthInfo {
  status: string;
  version: string;
  env?: string;
  deployed_at?: string;
  db: string;
}

export async function getHealth(): Promise<HealthInfo> {
  const res = await fetch(`${API_BASE_URL}/health`, { cache: "no-store" });
  if (!res.ok) {
    throw new Error(`Health check failed: ${res.status}`);
  }
  return res.json() as Promise<HealthInfo>;
}
