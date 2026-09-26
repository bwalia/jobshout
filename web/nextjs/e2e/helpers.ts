import { type Page, expect } from "@playwright/test";

const API_URL = process.env.E2E_API_URL ?? "http://localhost:8090/api/v1";

/** Generate a unique email for each call. */
export function uniqueEmail(prefix: string): string {
  return `${prefix}-${Date.now()}-${Math.random().toString(36).slice(2, 7)}@jobshout.io`;
}

/** Register a fresh user via API and return { token, email, password }. */
export async function registerViaAPI(prefix = "e2e"): Promise<{
  token: string;
  email: string;
  password: string;
}> {
  const email = uniqueEmail(prefix);
  const password = "testpass1234";
  const res = await fetch(`${API_URL}/auth/register`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      email,
      password,
      full_name: "E2E Tester",
      org_name: `E2E Org ${Date.now()}`,
    }),
  });
  if (!res.ok) {
    throw new Error(`Registration failed: ${res.status} ${await res.text()}`);
  }
  const data = await res.json();
  return { token: data.access_token, email, password };
}

/** Log in through the UI and wait for chat home. */
export async function loginViaUI(
  page: Page,
  email: string,
  password: string,
): Promise<void> {
  await page.goto("/login");
  await page.fill("#email", email);
  await page.fill("#password", password);
  await page.click('button[type="submit"]');
  await page.waitForURL("**/chat**", { timeout: 10_000 });
  await expect(page.getByRole("button", { name: /new chat/i })).toBeVisible();
}

/** Navigate by URL; old routes redirect to their new panel homes. */
export async function navigateTo(page: Page, href: string): Promise<void> {
  await page.goto(href);
  await page.waitForLoadState("domcontentloaded");
}

/** Create an agent via API and return its ID. */
export async function createAgentViaAPI(
  token: string,
  overrides: Record<string, string> = {},
): Promise<string> {
  const payload = {
    name: overrides.name ?? "E2E Test Agent",
    role: overrides.role ?? "tester",
    description: overrides.description ?? "Agent created by E2E tests",
    model_provider: "ollama",
    model_name: "llama3",
    system_prompt: "You are a helpful test agent.",
    engine_type: "go_native",
  };
  const res = await fetch(`${API_URL}/agents/`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${token}`,
    },
    body: JSON.stringify(payload),
  });
  if (!res.ok) {
    throw new Error(`Create agent failed: ${res.status} ${await res.text()}`);
  }
  return (await res.json()).id;
}

/** Create a project via API and return its ID. */
export async function createProjectViaAPI(
  token: string,
  overrides: Record<string, string> = {},
): Promise<string> {
  const payload = {
    name: overrides.name ?? "E2E Test Project",
    description: overrides.description ?? "Project created by E2E tests",
    status: "active",
    priority: "medium",
  };
  const res = await fetch(`${API_URL}/projects/`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${token}`,
    },
    body: JSON.stringify(payload),
  });
  if (!res.ok) {
    throw new Error(
      `Create project failed: ${res.status} ${await res.text()}`,
    );
  }
  return (await res.json()).id;
}

/** Create a task via API and return the created task. */
export async function createTaskViaAPI(
  token: string,
  projectId: string,
  overrides: {
    title?: string;
    description?: string;
    status?: string;
  } = {}
): Promise<{ id: string; title: string; status: string }> {
  const payload = {
    project_id: projectId,
    title: overrides.title ?? "E2E Task",
    description: overrides.description ?? "Created by E2E tests",
    status: overrides.status ?? "todo",
  };
  const res = await fetch(`${API_URL}/tasks/`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${token}`,
    },
    body: JSON.stringify(payload),
  });
  if (!res.ok) {
    throw new Error(`Create task failed: ${res.status} ${await res.text()}`);
  }
  return res.json();
}
