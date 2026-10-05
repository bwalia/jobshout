import { test, expect } from "@playwright/test";
import { registerViaAPI, loginViaUI } from "./helpers";

let creds: { email: string; password: string; token: string };

test.describe("Navigation & Layout", () => {
  test.beforeAll(async () => {
    creds = await registerViaAPI("nav");
  });

  test.beforeEach(async ({ page }) => {
    await loginViaUI(page, creds.email, creds.password);
  });

  const panels = [
    { href: "/panel/dashboard", heading: /Good (morning|afternoon|evening)/ },
    { href: "/panel/projects", heading: /Projects/ },
    { href: "/panel/tasks", heading: /Tasks/ },
    { href: "/panel/agents", heading: /All agents/ },
    { href: "/panel/artifacts", heading: /Outputs/ },
    { href: "/panel/workflows", heading: /Workflows/ },
    { href: "/panel/llm-providers", heading: /Models & providers/ },
  ];

  for (const { href, heading } of panels) {
    test(`opens panel ${href}`, async ({ page }) => {
      await page.goto(href);
      await page.waitForURL(`**${href}**`, { timeout: 10_000 });
      await expect(page.locator("h1").first()).toContainText(heading, {
        timeout: 5_000,
      });
    });
  }

  test("chat is home after login", async ({ page }) => {
    await expect(page).toHaveURL(/\/chat/);
    await expect(page.getByRole("button", { name: /new chat/i })).toBeVisible();
  });

  test("sidebar shows grouped sections, each item one click away", async ({ page }) => {
    const nav = page.getByRole("navigation", { name: "Main" });
    for (const section of ["Work", "Agents", "Automation", "Chats"]) {
      await expect(nav.getByRole("button", { name: section, exact: true })).toBeVisible();
    }
    for (const item of ["Home", "Projects", "Tasks", "Sprints", "Outputs", "All agents", "Org chart", "Skills & plugins", "Marketplace", "Workflows", "Schedules"]) {
      await expect(nav.getByRole("link", { name: item, exact: true })).toBeVisible();
    }
    const aside = page.locator("aside");
    await expect(aside.getByRole("link", { name: "Models & providers" })).toBeVisible();
    await expect(aside.getByRole("link", { name: "Settings" })).toBeVisible();

    await nav.getByRole("link", { name: "Tasks", exact: true }).click();
    await page.waitForURL("**/panel/tasks**");
    await expect(nav.getByRole("link", { name: "Tasks", exact: true })).toHaveAttribute("aria-current", "page");
  });

  test("a collapsed section stays collapsed after a reload", async ({ page }) => {
    const nav = page.getByRole("navigation", { name: "Main" });
    await nav.getByRole("button", { name: "Automation", exact: true }).click();
    await expect(nav.getByRole("link", { name: "Schedules" })).toHaveCount(0);
    await page.reload();
    await expect(nav.getByRole("button", { name: "Automation", exact: true })).toHaveAttribute("aria-expanded", "false");
    await expect(nav.getByRole("link", { name: "Schedules" })).toHaveCount(0);
    await nav.getByRole("button", { name: "Automation", exact: true }).click();
    await expect(nav.getByRole("link", { name: "Schedules" })).toBeVisible();
  });

  test("nested routes keep their nav item active, with breadcrumbs", async ({ page }) => {
    const res = await fetch(`${process.env.E2E_API_URL ?? "http://localhost:8090/api/v1"}/agents?per_page=100`, {
      headers: { Authorization: `Bearer ${creds.token}` },
    });
    const writer = (await res.json()).data.find(
      (a: { metadata?: { builtin?: string } }) => a.metadata?.builtin === "article_writer"
    );
    await page.goto(`/panel/agents/${writer.id}?tab=models`);
    const nav = page.getByRole("navigation", { name: "Main" });
    await expect(nav.getByRole("link", { name: "All agents" })).toHaveAttribute("aria-current", "page");
    await expect(page.getByRole("navigation", { name: "Breadcrumb" })).toContainText(/All agents[\s\S]*Article Writer[\s\S]*Models/);
  });

  test("old Task Board and Task Manager URLs redirect", async ({ page }) => {
    await page.goto("/panel/task-board?view=agents");
    await page.waitForURL(/\/panel\/tasks\?.*group=agent/, { timeout: 10_000 });
    await page.goto("/panel/task-manager?agent=mail&connected=1");
    await page.waitForURL(/\/panel\/agents\/[0-9a-f-]{36}\?tab=workspace.*connected=1/, { timeout: 15_000 });
    await page.goto("/panel/llm-benchmarks");
    await page.waitForURL(/\/panel\/llm-providers\?tab=benchmarks/, { timeout: 10_000 });
  });

  test("command palette jumps to a page", async ({ page }) => {
    await page.keyboard.press(process.platform === "darwin" ? "Meta+k" : "Control+k");
    const box = page.getByRole("combobox", { name: "Search" });
    await expect(box).toBeVisible();
    await box.fill("schedules");
    await page.keyboard.press("Enter");
    await page.waitForURL("**/panel/scheduler**", { timeout: 10_000 });
  });
});
