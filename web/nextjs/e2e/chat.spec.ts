import { test, expect } from "@playwright/test";
import { registerViaAPI, loginViaUI } from "./helpers";

test.describe("Chat", () => {
  test("sidebar opens chat page with empty state", async ({ page }) => {
    const creds = await registerViaAPI("chat");
    await loginViaUI(page, creds.email, creds.password);
    const link = page.locator('nav a[href="/chat"]');
    await expect(link).toBeVisible({ timeout: 5_000 });
    await link.click();
    await page.waitForURL("**/chat", { timeout: 10_000 });
    await expect(page.locator("h1").first()).toContainText("Chat");
    await expect(page.getByText("JobShout AI")).toBeVisible();
    await expect(
      page.getByRole("button", { name: "List my agents" })
    ).toBeVisible();
  });

  test("dock opens with keyboard shortcut", async ({ page }) => {
    const creds = await registerViaAPI("chatdock");
    await loginViaUI(page, creds.email, creds.password);
    await page.keyboard.press("ControlOrMeta+k");
    await expect(page.getByRole("dialog", { name: "JobShout chat" })).toBeVisible({
      timeout: 5_000,
    });
  });
});
