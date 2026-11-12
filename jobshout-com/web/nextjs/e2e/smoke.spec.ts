import { test, expect } from "@playwright/test";

test("home page loads JobShout branding", async ({ page }) => {
  await page.goto("/");
  await expect(page.locator("body")).toContainText(/JobShout/i);
});

test("applications redirects unsigned users to login", async ({ page }) => {
  await page.goto("/applications");
  await expect(page).toHaveURL(/login/i);
});
