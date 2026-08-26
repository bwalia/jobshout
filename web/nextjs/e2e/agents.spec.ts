import { test, expect } from "@playwright/test";
import { registerViaAPI, loginViaUI, navigateTo } from "./helpers";

let creds: { email: string; password: string; token: string };

test.describe("Agents (Task Manager panel)", () => {
  test.beforeAll(async () => {
    creds = await registerViaAPI("agents");
  });

  test.beforeEach(async ({ page }) => {
    await loginViaUI(page, creds.email, creds.password);
  });

  test("old /agents route lands in Task Manager", async ({ page }) => {
    await navigateTo(page, "/agents");
    await page.waitForURL("**/panel/task-manager**", { timeout: 10_000 });
    await expect(page.locator("h1")).toContainText("Task Manager");
  });

  test("create agent via dialog", async ({ page }) => {
    await navigateTo(page, "/panel/task-manager");

    await page.click('button:has-text("New agent")');
    await expect(page.locator('[role="dialog"]')).toBeVisible();

    await page.fill("#agent-name", "Playwright Test Agent");
    await page.fill("#agent-role", "e2e-tester");
    await page.fill("#agent-description", "Created by Playwright E2E test");
    // The model list is environment-dependent; assert on structure and pick
    // the last real option.
    const modelPicker = page.locator("#agent-model");
    await expect(modelPicker).toBeVisible();
    const optionCount = await modelPicker.locator("option").count();
    expect(optionCount).toBeGreaterThan(0);
    await modelPicker.selectOption({ index: optionCount - 1 });

    await page.fill(
      "#agent-system-prompt",
      "You are a test agent for E2E testing.",
    );

    await page.click('button[type="submit"]:has-text("Create Agent")');

    await expect(page.locator('[role="dialog"]')).not.toBeVisible({
      timeout: 5_000,
    });

    // Agent should appear in the master rail
    await expect(
      page.locator("text=Playwright Test Agent").first(),
    ).toBeVisible({ timeout: 5_000 });
  });

  test("agent detail shows and links to full profile", async ({ page }) => {
    await navigateTo(page, "/panel/task-manager");

    await page
      .locator('button:has-text("Playwright Test Agent")')
      .first()
      .click();
    await expect(page.locator("text=e2e-tester").first()).toBeVisible({
      timeout: 5_000,
    });

    await page.click('a:has-text("Full profile")');
    await page.waitForURL("**/agents/**", { timeout: 5_000 });
    await expect(page.locator("text=Overview")).toBeVisible();
  });

  test("create agent validation - empty name shows error", async ({
    page,
  }) => {
    await navigateTo(page, "/panel/task-manager");

    await page.click('button:has-text("New agent")');
    await expect(page.locator('[role="dialog"]')).toBeVisible();

    await page.fill("#agent-role", "tester");
    await page.click('button[type="submit"]:has-text("Create Agent")');

    // Dialog should remain open (validation failed)
    await expect(page.locator('[role="dialog"]')).toBeVisible();
  });
});
