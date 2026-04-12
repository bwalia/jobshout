import { test, expect } from "@playwright/test";
import { registerViaAPI, loginViaUI, navigateTo, createAgentViaAPI } from "./helpers";

let creds: { email: string; password: string; token: string };
let agentId: string;

test.describe("Workflows", () => {
  test.beforeAll(async () => {
    creds = await registerViaAPI("wf");
    agentId = await createAgentViaAPI(creds.token, {
      name: "Workflow Test Agent",
      role: "processor",
    });
  });

  test.beforeEach(async ({ page }) => {
    await loginViaUI(page, creds.email, creds.password);
  });

  test("workflows page loads", async ({ page }) => {
    await navigateTo(page, "/workflows");
    await expect(page.locator("h1")).toContainText("Workflows");
  });

  test("navigate to new workflow page", async ({ page }) => {
    await navigateTo(page, "/workflows");

    await page.click('button:has-text("New Workflow")');
    await page.waitForURL("**/workflows/new", { timeout: 5_000 });

    await expect(
      page.locator('input[placeholder="Workflow name"]'),
    ).toBeVisible({ timeout: 5_000 });
  });

  test("create workflow via API and verify it appears in list", async ({
    page,
  }) => {
    const res = await fetch("http://localhost:8090/api/v1/workflows/", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${creds.token}`,
      },
      body: JSON.stringify({
        name: "E2E Test Workflow",
        description: "A simple test workflow created by Playwright",
        steps: [
          {
            name: "step_one",
            agent_id: agentId,
            input_template: "Process this: {{.Input.data}}",
            position: 0,
            depends_on: [],
          },
        ],
      }),
    });
    expect(res.ok).toBeTruthy();

    await navigateTo(page, "/workflows");
    await expect(
      page.locator("text=E2E Test Workflow").first(),
    ).toBeVisible({ timeout: 5_000 });
  });

  test("search workflows", async ({ page }) => {
    await navigateTo(page, "/workflows");

    await page.fill('input[type="search"]', "E2E Test");
    await page.waitForTimeout(500);

    await expect(
      page.locator("text=E2E Test Workflow").first(),
    ).toBeVisible({ timeout: 5_000 });
  });
});
