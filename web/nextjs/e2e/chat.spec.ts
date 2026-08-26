import { test, expect } from "@playwright/test";
import { registerViaAPI, loginViaUI } from "./helpers";

test.describe("Chat", () => {
  test("home is chat with empty state", async ({ page }) => {
    const creds = await registerViaAPI("chat");
    await loginViaUI(page, creds.email, creds.password);
    await expect(page).toHaveURL(/\/chat/);
    await expect(page.getByText("JobShout").first()).toBeVisible();
    await expect(
      page.getByRole("button", { name: "List my agents" })
    ).toBeVisible();
  });

  test("command palette opens with keyboard shortcut", async ({ page }) => {
    const creds = await registerViaAPI("chatdock");
    await loginViaUI(page, creds.email, creds.password);
    await page.keyboard.press("ControlOrMeta+k");
    await expect(
      page.getByRole("dialog", { name: "Command palette" })
    ).toBeVisible({ timeout: 5_000 });
  });
});
