import { test, expect } from "@playwright/test";
import { registerViaAPI, loginViaUI, navigateTo, createProjectViaAPI } from "./helpers";

let creds: { email: string; password: string; token: string };

test.describe("New task launch fields", () => {
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
    await navigateTo(page, "/panel/tasks");
    await expect(page.locator("h1")).toContainText("Tasks");

    await page.getByRole("main").getByRole("button", { name: "New task" }).click();
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
    await page.locator("#agent-field-senders").fill("ops@example.com");
    await page.locator("#agent-field-senders").press("Enter");
    await expect(page.getByRole("button", { name: "Remove ops@example.com" })).toBeVisible();
    await expect(page.locator("#agent-field-knowledge_notes")).toBeVisible();
    await expect(page.locator("#agent-field-knowledge_urls")).toBeVisible();

    await pickAgent("Image Generator");
    await expect(page.locator("#agent-field-prompt")).toBeVisible();
  });

  test("Run on a created task reuses the saved topic instead of a blank form", async ({
    page,
  }) => {
    await navigateTo(page, "/panel/tasks");
    await page.getByRole("main").getByRole("button", { name: "New task" }).click();
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

  test("Article Writer namespace asks for a key only when the namespace is new", async ({
    page,
  }) => {
    // The server's answers are stubbed: which namespaces are connected is
    // per ring, and the key must never be needed for a connected one.
    const connected = new Set(["jobshout"]);
    await page.route("**/blogs/destinations/check**", async (route) => {
      const ns = new URL(route.request().url()).searchParams.get("namespace") ?? "";
      await route.fulfill({
        json: {
          namespace: ns,
          base_url: "https://opsapi.example.com",
          exists: connected.has(ns),
          ready: connected.has(ns),
          ...(connected.has(ns) ? { name: ns, integration_id: crypto.randomUUID() } : {}),
        },
      });
    });
    let created: Record<string, unknown> | null = null;
    await page.route("**/api/v1/integrations", async (route) => {
      if (route.request().method() !== "POST") return route.fallback();
      created = route.request().postDataJSON();
      connected.add(String((created?.config as Record<string, string>)?.namespace_slug));
      await route.fulfill({ status: 201, json: { id: crypto.randomUUID(), name: created?.name } });
    });

    await navigateTo(page, "/panel/tasks");
    await page.getByRole("main").getByRole("button", { name: "New task" }).click();
    const agentSelect = page.locator("#create-task-agent");
    const value = await agentSelect
      .locator("option", { hasText: "Article Writer" })
      .first()
      .getAttribute("value");
    await agentSelect.selectOption(value!);

    const ns = page.locator("#agent-field-destination");
    const key = page.locator("#agent-field-destination-api-key");

    // Existing namespace (the default): reused, no key field.
    await expect(ns).toHaveValue("jobshout");
    await expect(page.getByTestId("website-connected")).toBeVisible();
    await expect(key).toHaveCount(0);

    // New namespace: the key field appears, and connecting saves the key to
    // the integration — not to the task's launch values.
    await ns.fill("brand-new");
    await expect(key).toBeVisible();
    // Built at runtime so secret scanners don't read a fixture as a real key.
    await key.fill(["opsk", "e2e", "placeholder"].join("_"));
    await page.getByRole("button", { name: "Connect" }).click();
    await expect(page.getByTestId("website-connected")).toBeVisible();
    await expect(key).toHaveCount(0);
    expect(created).toMatchObject({
      name: "brand-new",
      provider: "opsapi",
      config: { namespace_slug: "brand-new" },
    });
    await expect(ns).toHaveValue("brand-new");
  });
});
