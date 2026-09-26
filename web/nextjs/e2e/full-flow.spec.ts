import { test, expect } from "@playwright/test";
import { uniqueEmail } from "./helpers";

/**
 * Full end-to-end flow: signup -> create agent -> create project ->
 * create tasks -> verify dashboard.
 */

const user = {
  fullName: "Full Flow Tester",
  email: uniqueEmail("fullflow"),
  orgName: `Full Flow Org ${Date.now()}`,
  password: "testpass1234",
};

test.describe("Full E2E Flow", () => {
  test("complete user journey: signup through task management", async ({
    page,
  }) => {
    // ── Step 1: Sign Up ──
    await page.goto("/signup");
    await page.fill("#fullName", user.fullName);
    await page.fill("#email", user.email);
    await page.fill("#orgName", user.orgName);
    await page.fill("#password", user.password);
    await page.click('button[type="submit"]');
    await page.waitForURL("**/chat**", { timeout: 15_000 });
    await expect(page.getByRole("button", { name: /new chat/i })).toBeVisible();

    // ── Step 2: Task Manager — create agent ──
    await page.goto("/panel/task-manager");
    await expect(page.locator("h1").first()).toContainText("Task Manager");

    await page.click('button:has-text("New agent")');
    await expect(page.locator('[role="dialog"]')).toBeVisible();

    await page.fill("#agent-name", "Flow Test Agent");
    await page.fill("#agent-role", "assistant");
    await page.fill("#agent-description", "Full flow test agent");
    await page.fill("#agent-system-prompt", "You help with testing.");

    await page.click('button[type="submit"]:has-text("Create Agent")');
    await expect(page.locator('[role="dialog"]')).not.toBeVisible({
      timeout: 5_000,
    });

    // ── Step 3: Create a Project via dialog ──
    await page.click('button:has-text("New project")');
    await page.fill("#project-name", "Flow Test Project");
    await page.fill("#project-desc", "Full E2E test project");
    await page.selectOption("#project-priority", "high");
    await page.click('button[type="submit"]:has-text("Create Project")');
    await expect(page.getByText("Flow Test Project").first()).toBeVisible({
      timeout: 8_000,
    });

    // ── Step 4: Create a task ──
    await page.click('button:has-text("New task")');
    await expect(page.getByText(/new task|create task/i).first()).toBeVisible({
      timeout: 5_000,
    });

    // ── Step 5: Verify dashboard panel ──
    await page.goto("/panel/dashboard");
    await page.waitForURL("**/panel/dashboard**", { timeout: 5_000 });
    await expect(page.locator("h1").first()).toContainText(/Good (morning|afternoon|evening)/);
    await expect(page.getByText("Task throughput")).toBeVisible();
  });
});
