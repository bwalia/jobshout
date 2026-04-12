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

const TASK_DIALOG = 'h2:has-text("New Task")';

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
    await page.waitForURL("**/dashboard", { timeout: 15_000 });
    await expect(page.locator("h1")).toContainText("Dashboard");

    // ── Step 2: Create an Agent ──
    await page.click('nav a[href="/agents"]');
    await page.waitForURL("**/agents", { timeout: 5_000 });

    await page.click('button:has-text("New Agent")');
    await expect(page.locator('[role="dialog"]')).toBeVisible();

    await page.fill("#agent-name", "Flow Test Agent");
    await page.fill("#agent-role", "assistant");
    await page.fill("#agent-description", "Full flow test agent");
    await page.fill("#agent-system-prompt", "You help with testing.");

    await page.click('button[type="submit"]:has-text("Create Agent")');
    await expect(page.locator('[role="dialog"]')).not.toBeVisible({
      timeout: 5_000,
    });
    await expect(page.locator("text=Flow Test Agent").first()).toBeVisible({
      timeout: 5_000,
    });

    // ── Step 3: Create a Project ──
    await page.click('nav a[href="/projects"]');
    await page.waitForURL("**/projects", { timeout: 5_000 });

    await page.click('button:has-text("New Project")');
    await page.fill("#project-name", "Flow Test Project");
    await page.fill("#project-desc", "Full E2E test project");
    await page.selectOption("#project-priority", "High");
    await page.click('button[type="submit"]:has-text("Create Project")');

    await expect(page.locator("text=Flow Test Project").first()).toBeVisible({
      timeout: 5_000,
    });

    // ── Step 4: Navigate into project and create tasks ──
    await page.click("text=Flow Test Project");
    await page.waitForURL("**/projects/**", { timeout: 5_000 });
    await expect(page.locator("text=Backlog").first()).toBeVisible({
      timeout: 5_000,
    });

    // Create first task
    await page.locator('button:has-text("Add task")').first().click();
    await expect(page.locator(TASK_DIALOG)).toBeVisible({ timeout: 5_000 });
    await page.fill("#create-task-title", "Setup CI/CD Pipeline");
    await page.fill("#create-task-desc", "Configure GitHub Actions");
    await page.selectOption("#create-task-priority", "High");
    await page.click('button:has-text("Create Task")');
    await expect(page.locator(TASK_DIALOG)).not.toBeVisible({
      timeout: 5_000,
    });
    await expect(
      page.locator("text=Setup CI/CD Pipeline").first(),
    ).toBeVisible({ timeout: 5_000 });

    // Create second task
    await page.locator('button:has-text("Add task")').first().click();
    await expect(page.locator(TASK_DIALOG)).toBeVisible({ timeout: 5_000 });
    await page.fill("#create-task-title", "Write API Documentation");
    await page.fill("#create-task-desc", "Document all REST endpoints");
    await page.selectOption("#create-task-priority", "Medium");
    await page.click('button:has-text("Create Task")');
    await expect(page.locator(TASK_DIALOG)).not.toBeVisible({
      timeout: 5_000,
    });
    await expect(
      page.locator("text=Write API Documentation").first(),
    ).toBeVisible({ timeout: 5_000 });

    // ── Step 5: Verify dashboard ──
    await page.click('nav a[href="/dashboard"]');
    await page.waitForURL("**/dashboard", { timeout: 5_000 });
    await expect(page.locator("h1")).toContainText("Dashboard");
  });
});
