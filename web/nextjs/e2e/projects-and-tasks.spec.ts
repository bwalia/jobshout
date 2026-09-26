import { test, expect } from "@playwright/test";
import {
  registerViaAPI,
  loginViaUI,
  navigateTo,
  createProjectViaAPI,
  createTaskViaAPI,
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

  test("old /projects route lands in Projects panel", async ({ page }) => {
    await navigateTo(page, "/projects");
    await page.waitForURL("**/panel/projects**", { timeout: 10_000 });
    await expect(page.locator("h1")).toContainText("Projects");
  });

  test("create a new project from the Projects panel", async ({ page }) => {
    await navigateTo(page, "/panel/projects");

    await page.click('button:has-text("New Project")');

    await page.fill("#project-name", "E2E Test Project");
    await page.fill("#project-desc", "Created by Playwright");
    await page.selectOption("#project-priority", "high");

    await page.click('button[type="submit"]:has-text("Create Project")');

    await expect(page.locator("h1")).toContainText("E2E Test Project", {
      timeout: 5_000,
    });
  });

  test("clicking a project opens its board and tasks", async ({ page }) => {
    await navigateTo(page, "/panel/projects");
    await expect(page.locator("h1")).toContainText("Projects");
    await page.getByRole("heading", { name: "E2E Kanban Project" }).click();
    await page.waitForURL(new RegExp(`project=${projectId}`), {
      timeout: 10_000,
    });
    await expect(page.locator("h1")).toContainText("E2E Kanban Project");
    await expect(page.locator("text=Backlog").first()).toBeVisible({
      timeout: 5_000,
    });

    await page.click('button:has-text("Tasks")');
    await expect(page).toHaveURL(/view=tasks/);
  });

  test("clicking a project opens its board and tasks", async ({ page }) => {
    await navigateTo(page, "/panel/projects");
    await expect(page.locator("h1")).toContainText("Projects");
    await page.getByRole("heading", { name: "E2E Kanban Project" }).click();
    await page.waitForURL(new RegExp(`project=${projectId}`), {
      timeout: 10_000,
    });
    await expect(page.locator("h1")).toContainText("E2E Kanban Project");
    await expect(page.locator("text=Backlog").first()).toBeVisible({
      timeout: 5_000,
    });

    await page.click('button:has-text("Tasks")');
    await expect(page).toHaveURL(/view=tasks/);
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
    await page.locator('button:has-text("Add a task")').first().click();

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

    await page.locator('button:has-text("Add a task")').first().click();
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

  test("done task shows history and hides Run in task manager", async ({
    page,
  }) => {
    const title = `Done history ${Date.now()}`;
    await createTaskViaAPI(creds.token, projectId, {
      title,
      status: "done",
    });

    await navigateTo(page, "/panel/task-manager");
    await expect(page.locator("h1")).toContainText("Task Manager");
    await page.getByRole("button", { name: title }).click();

    await expect(
      page.getByRole("button", { name: "Show History" }).first()
    ).toBeVisible();
    await expect(page.getByRole("button", { name: /^Run$/ })).toHaveCount(0);

    await page.getByRole("button", { name: "Show History" }).first().click();
    await expect(page.getByRole("heading", { name: "History" })).toBeVisible();
    await expect(page.getByText(/Completed/i).first()).toBeVisible();
  });

  test("task board detail offers Run and Show History", async ({ page }) => {
    const title = `Board run ${Date.now()}`;
    await createTaskViaAPI(creds.token, projectId, {
      title,
      status: "todo",
    });

    await navigateTo(page, "/panel/task-board");
    await expect(page.locator("h1")).toContainText("Task Board");
    await page.getByRole("button", { name: new RegExp(title) }).click();
    await expect(page.getByRole("heading", { name: "Task Detail" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Show History" })).toBeVisible();
    await expect(page.getByRole("button", { name: /^Run$/ })).toBeVisible();
  });

  test("kanban card click writes task on the board URL", async ({ page }) => {
    const title = `Kanban url ${Date.now()}`;
    const task = await createTaskViaAPI(creds.token, projectId, {
      title,
      status: "todo",
    });

    await navigateTo(page, `/panel/task-board?project=${projectId}`);
    await expect(page.locator("h1")).toContainText("Task Board");
    await page.getByText(title, { exact: true }).first().click();
    await expect(page).toHaveURL(new RegExp(`task=${task.id}`));
    await expect(page.getByRole("heading", { name: "Task Detail" })).toBeVisible();
  });

  test("task manager click writes project and task on the URL", async ({
    page,
  }) => {
    const title = `TM url ${Date.now()}`;
    const task = await createTaskViaAPI(creds.token, projectId, {
      title,
      status: "todo",
    });

    await navigateTo(page, "/panel/task-manager");
    await expect(page.locator("h1")).toContainText("Task Manager");
    await page.getByRole("button", { name: title }).click();
    await expect(page).toHaveURL(new RegExp(`project=${projectId}`));
    await expect(page).toHaveURL(new RegExp(`task=${task.id}`));
  });
});
