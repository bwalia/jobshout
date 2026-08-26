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
    { href: "/panel/task-board", heading: /Task Board/ },
    { href: "/panel/task-manager", heading: /Task Manager/ },
    { href: "/panel/workflows", heading: /Workflows/ },
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
});
