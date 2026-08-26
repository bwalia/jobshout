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

  test("old /projects route lands in Task Manager", async ({ page }) => {
    await navigateTo(page, "/projects");
    await page.waitForURL("**/panel/task-manager**", { timeout: 10_000 });
    await expect(page.locator("h1")).toContainText("Task Manager");
  });

  test("create a new project via dialog", async ({ page }) => {
    await navigateTo(page, "/panel/task-manager");

    await page.click('button:has-text("New project")');

    await page.fill("#project-name", "E2E Test Project");
    await page.fill("#project-desc", "Created by Playwright");
    await page.selectOption("#project-priority", "high");

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

  test("task manager panel loads with project rail", async ({ page }) => {
    await navigateTo(page, "/task-manager");
    await page.waitForURL("**/panel/task-manager**", { timeout: 10_000 });
    await expect(page.locator("h1")).toContainText("Task Manager");
    await expect(page.locator("text=Projects").first()).toBeVisible({
      timeout: 5_000,
    });
  });

  test("task board shows all-tasks kanban and agents view", async ({ page }) => {
    await navigateTo(page, "/panel/task-board");
    await expect(page.locator("h1")).toContainText("Task Board");
    await expect(page.locator("text=Backlog").first()).toBeVisible({
      timeout: 5_000,
    });

    await page.click('button:has-text("Agents")');
    await page.waitForURL("**view=agents**", { timeout: 5_000 });
  });
});
