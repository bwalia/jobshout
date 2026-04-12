import { test, expect } from "@playwright/test";
import {
  registerViaAPI,
  loginViaUI,
  navigateTo,
  createProjectViaAPI,
} from "./helpers";

let creds: { email: string; password: string; token: string };
let projectId: string;

// Selector for the kanban task dialog (custom div, no role="dialog")
const TASK_DIALOG = 'h2:has-text("New Task")';

test.describe("Projects & Tasks", () => {
  test.beforeAll(async () => {
    creds = await registerViaAPI("proj");
    projectId = await createProjectViaAPI(creds.token, {
      name: "E2E Kanban Project",
    });
  });

  test.beforeEach(async ({ page }) => {
    await loginViaUI(page, creds.email, creds.password);
  });

  test("projects page loads", async ({ page }) => {
    await navigateTo(page, "/projects");
    await expect(page.locator("h1")).toContainText("Projects");
  });

  test("create a new project via dialog", async ({ page }) => {
    await navigateTo(page, "/projects");

    await page.click('button:has-text("New Project")');

    await page.fill("#project-name", "E2E Test Project");
    await page.fill("#project-desc", "Created by Playwright");
    await page.selectOption("#project-priority", "High");

    await page.click('button[type="submit"]:has-text("Create Project")');

    await expect(
      page.locator("text=E2E Test Project").first(),
    ).toBeVisible({ timeout: 5_000 });
  });

  test("navigate to project detail and see kanban board", async ({ page }) => {
    await page.goto(`/projects/${projectId}`);

    await expect(page.locator("text=Backlog").first()).toBeVisible({
      timeout: 5_000,
    });
    await expect(page.locator("text=Todo").first()).toBeVisible();
    await expect(page.locator("text=In Progress").first()).toBeVisible();
  });

  test("create a task from kanban board", async ({ page }) => {
    await page.goto(`/projects/${projectId}`);
    await expect(page.locator("text=Backlog").first()).toBeVisible({
      timeout: 5_000,
    });

    // Click "Add task" button
    await page.locator('button:has-text("Add task")').first().click();

    // Wait for the custom task dialog to appear
    await expect(page.locator(TASK_DIALOG)).toBeVisible({ timeout: 5_000 });

    await page.fill("#create-task-title", "E2E Test Task - Build Login Page");
    await page.fill(
      "#create-task-desc",
      "Acceptance: user can log in with email and password",
    );
    await page.selectOption("#create-task-priority", "High");
    await page.click('button:has-text("Create Task")');

    // Dialog should close and task should appear
    await expect(page.locator(TASK_DIALOG)).not.toBeVisible({
      timeout: 5_000,
    });
    await expect(
      page.locator("text=E2E Test Task - Build Login Page").first(),
    ).toBeVisible({ timeout: 5_000 });
  });

  test("create second task and verify both visible", async ({ page }) => {
    await page.goto(`/projects/${projectId}`);
    await expect(page.locator("text=Backlog").first()).toBeVisible({
      timeout: 5_000,
    });

    await page.locator('button:has-text("Add task")').first().click();
    await expect(page.locator(TASK_DIALOG)).toBeVisible({ timeout: 5_000 });

    await page.fill("#create-task-title", "E2E Task - Write Unit Tests");
    await page.fill("#create-task-desc", "Cover auth and agent services");
    await page.selectOption("#create-task-priority", "Medium");
    await page.click('button:has-text("Create Task")');

    await expect(page.locator(TASK_DIALOG)).not.toBeVisible({
      timeout: 5_000,
    });
    await expect(
      page.locator("text=E2E Task - Write Unit Tests").first(),
    ).toBeVisible({ timeout: 5_000 });
  });

  test("task manager page loads with hierarchical view", async ({ page }) => {
    await navigateTo(page, "/task-manager");
    await expect(page.locator("h1")).toContainText("Multi-Level Task Manager");
    await expect(page.locator("text=Total Tasks").first()).toBeVisible({
      timeout: 5_000,
    });
  });

  test("add root task from task manager", async ({ page }) => {
    await navigateTo(page, "/task-manager");
    await expect(page.locator("h1")).toContainText("Multi-Level Task Manager");

    await page.click('button:has-text("Add Root Task")');

    const titleInput = page.locator('input[placeholder="New task title..."]');
    await expect(titleInput).toBeVisible({ timeout: 3_000 });
    await titleInput.fill("E2E Root Task");

    // Click the "Add" submit button (not "Add Root Task" toggle)
    await page.locator('button:text-is("Add")').click();

    // Wait for network response and check
    await page.waitForTimeout(2_000);
    // The task list should refresh - check if it appears
    await expect(page.locator("text=E2E Root Task").first()).toBeVisible({
      timeout: 10_000,
    });
  });
});
