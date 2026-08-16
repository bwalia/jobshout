import { test, expect } from "@playwright/test";
import { registerViaAPI, loginViaUI, navigateTo } from "./helpers";

let creds: { email: string; password: string; token: string };

test.describe("Agents", () => {
  test.beforeAll(async () => {
    creds = await registerViaAPI("agents");
  });

  test.beforeEach(async ({ page }) => {
    await loginViaUI(page, creds.email, creds.password);
  });

  test("agents page loads and shows heading", async ({ page }) => {
    await navigateTo(page, "/agents");
    await expect(page.locator("h1")).toContainText("Agents");
  });

  test("create agent via dialog", async ({ page }) => {
    await navigateTo(page, "/agents");

    await page.click('button:has-text("New Agent")');
    await expect(page.locator('[role="dialog"]')).toBeVisible();

    await page.fill("#agent-name", "Playwright Test Agent");
    await page.fill("#agent-role", "e2e-tester");
    await page.fill("#agent-description", "Created by Playwright E2E test");
    // The model list is discovered from whichever providers this machine has
    // configured, so it is environment-dependent by design. Assert on structure
    // rather than on a model name, and pick the last real option so the test
    // exercises a genuine selection without requiring a live Ollama.
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

    // Dialog should close
    await expect(page.locator('[role="dialog"]')).not.toBeVisible({
      timeout: 5_000,
    });

    // Agent should appear in list (use .first() to avoid matching toast)
    await expect(
      page.locator("text=Playwright Test Agent").first(),
    ).toBeVisible({ timeout: 5_000 });
  });

  test("search filters agents", async ({ page }) => {
    await navigateTo(page, "/agents");
    await page.waitForTimeout(1_000);

    await page.fill('input[type="search"]', "Playwright");
    await page.waitForTimeout(500);

    await expect(
      page.locator("text=Playwright Test Agent").first(),
    ).toBeVisible({ timeout: 5_000 });
  });

  test("agent detail page loads", async ({ page }) => {
    await navigateTo(page, "/agents");

    await page
      .locator('a:has-text("Playwright Test Agent")')
      .first()
      .click();
    await page.waitForURL("**/agents/**", { timeout: 5_000 });

    await expect(page.locator("text=Overview")).toBeVisible();
    // Use .first() to handle strict mode
    await expect(page.locator("text=e2e-tester").first()).toBeVisible();
  });

  test("create agent validation - empty name shows error", async ({
    page,
  }) => {
    await navigateTo(page, "/agents");

    await page.click('button:has-text("New Agent")');
    await expect(page.locator('[role="dialog"]')).toBeVisible();

    await page.fill("#agent-role", "tester");
    await page.click('button[type="submit"]:has-text("Create Agent")');

    // Dialog should remain open (validation failed)
    await expect(page.locator('[role="dialog"]')).toBeVisible();
  });
});
