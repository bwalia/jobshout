import { test, expect } from "@playwright/test";
import { readFile } from "fs/promises";
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
    await expect(page.getByRole("button", { name: "Export" })).toBeVisible();
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

  test("import and export controls are on Task Manager", async ({ page }) => {
    await navigateTo(page, "/panel/task-manager");
    await expect(page.getByRole("button", { name: "Import agent" })).toBeVisible();

    await page.locator('button:has-text("Playwright Test Agent")').first().click();
    await expect(page.getByRole("button", { name: "Export" })).toBeVisible();
  });

  test("export downloads a package without org_id or secrets", async ({
    page,
  }) => {
    await navigateTo(page, "/panel/task-manager");
    await page.locator('button:has-text("Playwright Test Agent")').first().click();
    await expect(page.getByRole("button", { name: "Export" })).toBeVisible();

    const [download] = await Promise.all([
      page.waitForEvent("download"),
      page.getByRole("button", { name: "Export" }).click(),
    ]);
    expect(download.suggestedFilename()).toMatch(/\.jobshout-agent\.json$/);
    const path = await download.path();
    expect(path).toBeTruthy();
    const raw = await readFile(path!, "utf8");
    expect(raw).not.toContain('"org_id"');
    const pkg = JSON.parse(raw) as {
      kind: string;
      schema_version: number;
      agent: { name: string; engine_config?: Record<string, unknown> };
      warnings?: string[];
    };
    expect(pkg.kind).toBe("jobshout.agent");
    expect(pkg.schema_version).toBe(1);
    expect(pkg.agent.name).toBe("Playwright Test Agent");
    expect(JSON.stringify(pkg.agent.engine_config ?? {})).not.toMatch(
      /api_key|secret|token|password/i,
    );
    expect(pkg.warnings?.length).toBeGreaterThan(0);
  });

  test("import rejects invalid files then creates a custom agent", async ({
    page,
  }) => {
    await navigateTo(page, "/panel/task-manager");
    await page.getByRole("button", { name: "Import agent" }).click();
    await expect(page.getByRole("dialog", { name: "Import agent" })).toBeVisible();

    await page.locator("#agent-pack-file").setInputFiles({
      name: "broken.json",
      mimeType: "application/json",
      buffer: Buffer.from("{"),
    });
    await expect(
      page.getByRole("dialog").getByText("That file is not valid JSON."),
    ).toBeVisible();

    await page.locator("#agent-pack-file").setInputFiles({
      name: "wrong-kind.json",
      mimeType: "application/json",
      buffer: Buffer.from(
        JSON.stringify({
          kind: "nope",
          schema_version: 1,
          agent: { name: "X", role: "Y" },
        }),
      ),
    });
    await expect(
      page.getByRole("dialog").getByText("not a JobShout agent package"),
    ).toBeVisible();

    const name = `Imported Pack Agent ${Date.now()}`;
    await page.locator("#agent-pack-file").setInputFiles({
      name: "pack.jobshout-agent.json",
      mimeType: "application/json",
      buffer: Buffer.from(
        JSON.stringify({
          kind: "jobshout.agent",
          schema_version: 1,
          agent: {
            name,
            role: "QA",
            description: "Imported by Playwright",
            system_prompt: "You verify imports.",
          },
          tools: ["http_request"],
        }),
      ),
    });
    await expect(page.getByRole("dialog").getByText(name)).toBeVisible({
      timeout: 10_000,
    });
    await expect(
      page.getByRole("dialog").getByRole("button", { name: "Import", exact: true }),
    ).toBeEnabled();
    await page
      .getByRole("dialog")
      .getByRole("button", { name: "Import", exact: true })
      .click();
    await expect(page.getByRole("dialog", { name: "Import agent" })).not.toBeVisible({
      timeout: 10_000,
    });
    await expect(page).toHaveURL(/agent=/);
    await expect(page.getByRole("heading", { name })).toBeVisible({
      timeout: 5_000,
    });
    await expect(page.getByRole("button", { name: "Remove" })).toBeVisible();
    page.once("dialog", (dialog) => dialog.accept());
    await page.getByRole("button", { name: "Remove" }).click();
    await expect(page.getByRole("heading", { name })).not.toBeVisible({
      timeout: 8_000,
    });
  });

  test("import of a seeded specialist shows overlay confirm copy", async ({
    page,
  }) => {
    await navigateTo(page, "/panel/task-manager");
    await page.getByRole("button", { name: "Import agent" }).click();
    await page.locator("#agent-pack-file").setInputFiles({
      name: "mail.jobshout-agent.json",
      mimeType: "application/json",
      buffer: Buffer.from(
        JSON.stringify({
          kind: "jobshout.agent",
          schema_version: 1,
          agent: {
            name: "Mail Agent",
            role: "Mail",
            builtin: "mail",
            system_prompt: "You draft mail after import.",
          },
        }),
      ),
    });
    await expect(
      page.getByRole("dialog").getByText(/already has Mail Agent/i).first(),
    ).toBeVisible({ timeout: 10_000 });
    await expect(
      page.getByRole("dialog").getByText(/cannot be undone from this dialog/i).first(),
    ).toBeVisible();
    await expect(page.getByRole("button", { name: "Update agent" })).toBeEnabled();
    await page.getByRole("button", { name: "Cancel" }).click();
  });

  test("specialist tab has export and no remove", async ({ page }) => {
    await navigateTo(page, "/panel/task-manager?agent=mail");
    await expect(page.getByRole("heading", { name: "Mail Agent" })).toBeVisible({
      timeout: 10_000,
    });
    await expect(page.getByRole("button", { name: "Export" })).toBeVisible();
    await expect(page.getByRole("button", { name: "Remove" })).toHaveCount(0);
  });
});
