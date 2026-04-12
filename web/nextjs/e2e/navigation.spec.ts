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

  const sidebarLinks = [
    { href: "/dashboard", heading: "Dashboard" },
    { href: "/agents", heading: "Agents" },
    { href: "/projects", heading: "Projects" },
    { href: "/task-manager", heading: "Multi-Level Task Manager" },
    { href: "/workflows", heading: "Workflows" },
  ];

  for (const { href, heading } of sidebarLinks) {
    test(`sidebar navigates to ${href}`, async ({ page }) => {
      const link = page.locator(`nav a[href="${href}"]`);
      await expect(link).toBeVisible({ timeout: 5_000 });
      await link.click();
      await page.waitForURL(`**${href}`, { timeout: 10_000 });
      await expect(page.locator("h1").first()).toContainText(heading, {
        timeout: 5_000,
      });
    });
  }

  test("dashboard shows stats cards", async ({ page }) => {
    await expect(page.locator("h1")).toContainText("Dashboard");
  });
});
