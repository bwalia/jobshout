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
    await expect(page.locator("text=Backlog >> visible=true").first()).toBeVisible({
      timeout: 5_000,
    });

    // The project's tasks are the shared Tasks view: Board or List.
    await page.getByRole("button", { name: "List" }).click();
    await expect(page.getByRole("button", { name: "List" })).toHaveAttribute("aria-pressed", "true");
  });

  test("navigate to project detail and see kanban board", async ({ page }) => {
    await page.goto(`/projects/${projectId}`);

    await expect(page.locator("text=Backlog >> visible=true").first()).toBeVisible({
      timeout: 5_000,
    });
    await expect(page.locator("text=Todo >> visible=true").first()).toBeVisible();
    await expect(page.locator("text=In Progress >> visible=true").first()).toBeVisible();
  });

  test("create a task from kanban board", async ({ page }) => {
    await page.goto(`/projects/${projectId}`);
    await expect(page.locator("text=Backlog >> visible=true").first()).toBeVisible({
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
    await expect(page.locator("text=Backlog >> visible=true").first()).toBeVisible({
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

  test("old /task-manager lands on Tasks", async ({ page }) => {
    await navigateTo(page, "/task-manager");
    // Task Manager folded into Tasks and the agent profiles.
    await page.waitForURL("**/panel/tasks**", { timeout: 10_000 });
    await expect(page.locator("h1")).toContainText("Tasks");
  });

  test("tasks shows the all-tasks board, groups by agent and switches to a list", async ({ page }) => {
    await navigateTo(page, "/panel/tasks");
    await expect(page.locator("h1")).toContainText("Tasks");
    await expect(page.locator("text=Backlog >> visible=true").first()).toBeVisible({
      timeout: 5_000,
    });

    await page.getByRole("combobox", { name: "Group by" }).selectOption("agent");
    await page.waitForURL("**group=agent**", { timeout: 5_000 });
    await page.getByRole("button", { name: "List" }).click();
    await expect(page.getByRole("table")).toBeVisible();
    await page.getByRole("button", { name: "Board" }).click();
  });

  test("done task shows history and hides Run", async ({
    page,
  }) => {
    const title = `Done history ${Date.now()}`;
    await createTaskViaAPI(creds.token, projectId, {
      title,
      status: "done",
    });

    await navigateTo(page, `/panel/tasks?q=${encodeURIComponent(title)}`);
    await expect(page.locator("h1")).toContainText("Tasks");
    await page.getByText(title, { exact: true }).first().click();

    await expect(
      page.getByRole("button", { name: "Show History" }).first()
    ).toBeVisible();
    await expect(page.getByRole("button", { name: /^Run$/ })).toHaveCount(0);

    await page.getByRole("button", { name: "Show History" }).first().click();
    await expect(page.getByRole("heading", { name: "History" }).first()).toBeVisible();
    await expect(page.getByText(/Completed/i).first()).toBeVisible();
  });

  test("task detail offers Run and Show History", async ({ page }) => {
    const title = `Board run ${Date.now()}`;
    await createTaskViaAPI(creds.token, projectId, {
      title,
      status: "todo",
    });

    await navigateTo(page, `/panel/tasks?q=${encodeURIComponent(title)}`);
    await expect(page.locator("h1")).toContainText("Tasks");
    await page.getByText(title, { exact: true }).first().click();
    await expect(page.getByRole("heading", { name: "Task Detail" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Show History" })).toBeVisible();
    await expect(page.getByRole("button", { name: /^Run$/ })).toBeVisible();
  });

  test("kanban card click writes task on the Tasks URL", async ({ page }) => {
    const title = `Kanban url ${Date.now()}`;
    const task = await createTaskViaAPI(creds.token, projectId, {
      title,
      status: "todo",
    });

    // Old Task Board links carry the project over to Tasks.
    await navigateTo(page, `/panel/task-board?project=${projectId}`);
    await page.waitForURL(new RegExp(`/panel/tasks\\?.*project=${projectId}`), { timeout: 10_000 });
    await expect(page.locator("h1")).toContainText("Tasks");
    await page.getByText(title, { exact: true }).first().click();
    await expect(page).toHaveURL(new RegExp(`task=${task.id}`));
    await expect(page.getByRole("heading", { name: "Task Detail" })).toBeVisible();
  });

  test("filters and the open task live in the URL", async ({ page }) => {
    const title = `URL state ${Date.now()}`;
    const task = await createTaskViaAPI(creds.token, projectId, {
      title,
      status: "todo",
    });

    await navigateTo(page, "/panel/tasks");
    await page.getByRole("combobox", { name: "Project" }).selectOption(projectId);
    await expect(page).toHaveURL(new RegExp(`project=${projectId}`));
    await page.getByRole("searchbox", { name: "Search tasks" }).fill(title);
    await expect(page).toHaveURL(/q=URL/);
    await page.getByText(title, { exact: true }).first().click();
    await expect(page).toHaveURL(new RegExp(`task=${task.id}`));
  });
});
