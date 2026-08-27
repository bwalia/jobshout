import { test, expect } from "@playwright/test";
import { registerViaAPI, loginViaUI } from "./helpers";

test.describe("Chat", () => {
  test("home is chat with empty state", async ({ page }) => {
    const creds = await registerViaAPI("chat");
    await loginViaUI(page, creds.email, creds.password);
    await expect(page).toHaveURL(/\/chat/);
    await expect(page.getByRole("button", { name: "JobShout home" })).toBeVisible();
    const composer = page.getByPlaceholder(/ask jobshout to build/i);
    await expect(composer).toBeVisible();
    const box = await composer.boundingBox();
    const vh = page.viewportSize()?.height ?? 720;
    expect(box).toBeTruthy();
    expect(box!.y).toBeGreaterThan(vh * 0.2);
    expect(box!.y + box!.height).toBeLessThan(vh * 0.85);
    await expect(
      page.getByRole("button", { name: "List my agents" })
    ).toBeVisible();
    await expect(page.getByRole("link", { name: "Dashboard" })).toBeVisible();
    await expect(page.getByRole("link", { name: "Automations" })).toBeVisible();
    await expect(page.getByRole("link", { name: "Task Board" })).toHaveCount(0);
    await expect(page.locator("[data-chat-layout=hero]")).toBeVisible();
  });

  test("command palette opens with keyboard shortcut", async ({ page }) => {
    const creds = await registerViaAPI("chatdock");
    await loginViaUI(page, creds.email, creds.password);
    await page.keyboard.press("ControlOrMeta+k");
    await expect(
      page.getByRole("dialog", { name: "Command palette" })
    ).toBeVisible({ timeout: 5_000 });
  });

  test("first prompt docks the composer", async ({ page }) => {
    const creds = await registerViaAPI("chatdock2");
    await loginViaUI(page, creds.email, creds.password);
    await expect(page.locator("[data-chat-layout=hero]")).toBeVisible();
    await page.getByPlaceholder(/ask jobshout to build/i).fill("List my agents");
    await page.getByRole("button", { name: "Send message" }).click();
    await expect(page.locator("[data-chat-layout=docked]")).toBeVisible({
      timeout: 5_000,
    });
  });
});
