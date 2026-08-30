import { test, expect } from "@playwright/test";
import { registerViaAPI, loginViaUI, navigateTo, createProjectViaAPI } from "./helpers";

let creds: { email: string; password: string; token: string };

test.describe("Task Manager launch fields", () => {
  test.beforeAll(async () => {
    creds = await registerViaAPI("tm-launch");
    await createProjectViaAPI(creds.token, { name: "Launch Board" });
  });

  test.beforeEach(async ({ page }) => {
    await loginViaUI(page, creds.email, creds.password);
  });

  test("Article Writer, Research Agent, Mail Agent, and Image Generator show their fields", async ({
    page,
  }) => {
    await navigateTo(page, "/panel/task-manager");
    await expect(page.locator("h1")).toContainText("Task Manager");

    await page.click('button:has-text("New task")');
    await expect(page.getByRole("heading", { name: /new task/i })).toBeVisible({
      timeout: 8_000,
    });

    const agentSelect = page.locator("#create-task-agent");
    await expect(agentSelect).toBeVisible();

    async function pickAgent(name: string) {
      const value = await agentSelect
        .locator("option", { hasText: name })
        .first()
        .getAttribute("value");
      expect(value, `agent option for ${name}`).toBeTruthy();
      await agentSelect.selectOption(value!);
    }

    await pickAgent("Article Writer");
    await expect(page.locator("#agent-field-topic")).toBeVisible();
    await expect(page.locator("#agent-field-topic")).toHaveAttribute(
      "placeholder",
      /Edge AI/i,
    );

    await pickAgent("Research Agent");
    await expect(page.locator("#agent-field-topic")).toBeVisible();
    await expect(page.getByText(/cited findings/i).first()).toBeVisible();

    await pickAgent("Mail Agent");
    await expect(page.getByText("Who to watch").first()).toBeVisible({
      timeout: 8_000,
    });
    await expect(page.getByText("How to answer").first()).toBeVisible();
    await expect(page.locator("#agent-field-senders")).toBeVisible();
    await expect(page.locator("#agent-field-knowledge_notes")).toBeVisible();
    await expect(page.locator("#agent-field-knowledge_urls")).toBeVisible();

    await pickAgent("Image Generator");
    await expect(page.locator("#agent-field-prompt")).toBeVisible();
  });

  test("Run on a created task reuses the saved topic instead of a blank form", async ({
    page,
  }) => {
    await navigateTo(page, "/panel/task-manager");
    await page.click('button:has-text("New task")');
    const agentSelect = page.locator("#create-task-agent");
    const value = await agentSelect
      .locator("option", { hasText: "Research Agent" })
      .first()
      .getAttribute("value");
    await agentSelect.selectOption(value!);
    await page.locator("#agent-field-topic").fill("kubernetes cost optimisation");
    await page.getByRole("button", { name: "Create task" }).click();
    await expect(page.getByRole("heading", { name: /new task/i })).toHaveCount(0, {
      timeout: 8_000,
    });
    await expect(
      page.getByRole("heading", { name: /Research: kubernetes cost optimisation/i }),
    ).toBeVisible();

    await page.getByRole("button", { name: /^Run$/ }).click();
    await expect(page.getByRole("heading", { name: /run task with an agent/i })).toBeVisible();
    await expect(page.getByRole("heading", { name: /new task/i })).toHaveCount(0);
    await expect(page.locator("#agent-field-topic")).toHaveValue(
      "kubernetes cost optimisation",
    );
  });
});
