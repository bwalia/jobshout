import { test, expect } from "@playwright/test";
import { registerViaAPI, loginViaUI, navigateTo } from "./helpers";

const API_URL = process.env.E2E_API_URL ?? "http://localhost:8090/api/v1";

let creds: { email: string; password: string; token: string };

test.describe("Articles", () => {
  test.beforeAll(async () => {
    creds = await registerViaAPI("articles");
  });

  test.beforeEach(async ({ page }) => {
    await loginViaUI(page, creds.email, creds.password);
  });

  // The Article Writer is seeded for every new organization, so a brand-new
  // account has something on the dashboard rather than an empty state.
  test("a new org gets the built-in Article Writer agent", async ({ page }) => {
    await expect(page.getByText("Article Writer").first()).toBeVisible({
      timeout: 5_000,
    });

    const res = await fetch(`${API_URL}/agents`, {
      headers: { Authorization: `Bearer ${creds.token}` },
    });
    const body = await res.json();
    const writer = body.data.find(
      (a: { metadata?: Record<string, string> }) =>
        a.metadata?.builtin === "article_writer",
    );
    expect(writer).toBeTruthy();
    // 'active' is what puts it in the dashboard's Active Agents grid.
    expect(writer.status).toBe("active");
  });

  test("articles page loads with an empty state", async ({ page }) => {
    await navigateTo(page, "/articles");
    await expect(page.locator("h1")).toContainText("Articles");
    await expect(page.getByText("No articles yet.")).toBeVisible({
      timeout: 5_000,
    });
  });

  test("the generate dialog validates the topic list", async ({ page }) => {
    await navigateTo(page, "/articles");
    await page.getByRole("button", { name: "Write articles" }).first().click();

    // Nothing typed yet, so there is nothing to write.
    const submit = page.getByRole("button", { name: "Write", exact: true });
    await expect(submit).toBeDisabled();

    await page.fill("#topics", "First topic\nSecond topic");
    await expect(page.getByText("2 topics")).toBeVisible();
    await expect(submit).toBeEnabled();

    // The server truncates past the hard cap, so the form refuses first.
    await page.fill(
      "#topics",
      Array.from({ length: 11 }, (_, i) => `Topic ${i}`).join("\n"),
    );
    await expect(page.getByText("maximum is 10")).toBeVisible();
    await expect(submit).toBeDisabled();
  });

  // Generation needs a reachable LLM, so this drives the run through the API
  // and asserts the UI renders what came back — the parts that are ours.
  test("a completed run renders both the article and its markdown", async ({
    page,
  }) => {
    // Real generation against a real model; well past the default 60s.
    test.setTimeout(180_000);

    const start = await fetch(`${API_URL}/blogs/generate`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${creds.token}`,
      },
      body: JSON.stringify({ topics: ["End to end article test"] }),
    });
    expect(start.status).toBe(202);
    const run = await start.json();

    // The run is asynchronous; wait for it to settle before asserting. The
    // budget is generous because a large model that is not already resident
    // has to load before it emits anything.
    let final = run;
    for (let i = 0; i < 120 && final.status === "running"; i++) {
      await new Promise((r) => setTimeout(r, 1_000));
      final = await fetch(`${API_URL}/blogs/runs/${run.id}`, {
        headers: { Authorization: `Bearer ${creds.token}` },
      }).then((r) => r.json());
    }
    test.skip(
      final.status !== "completed",
      `generation did not complete (${final.status}) — needs a reachable LLM`,
    );

    await page.goto(`/articles/${run.id}`);

    await expect(page.getByText("End to end article test").first()).toBeVisible({
      timeout: 10_000,
    });
    // Every step in the trace should have finished.
    await expect(page.getByText("Queued")).toBeVisible();
    await expect(page.getByText(/Generated \d+ article/)).toBeVisible();

    // The rendered view produces real headings, not escaped markdown.
    await expect(page.locator("article h1").first()).toBeVisible();
    await expect(page.getByText("# End to end")).toHaveCount(0);

    // The raw view shows the markdown source verbatim.
    await page.getByRole("button", { name: "Markdown" }).click();
    await expect(page.locator("pre").first()).toContainText("#");
  });

  test("publish is unavailable when GitHub is not configured", async ({
    page,
  }) => {
    const config = await fetch(`${API_URL}/blogs/config`, {
      headers: { Authorization: `Bearer ${creds.token}` },
    }).then((r) => r.json());
    test.skip(config.can_publish, "GitHub is configured on this server");

    const start = await fetch(`${API_URL}/blogs/generate`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${creds.token}`,
      },
      body: JSON.stringify({ topics: ["Publish gating test"] }),
    });
    const run = await start.json();

    await page.goto(`/articles/${run.id}`);
    await expect(
      page.getByText("Publishing is unavailable"),
    ).toBeVisible({ timeout: 10_000 });
    await expect(page.getByRole("button", { name: /Publish/ })).toBeDisabled();
  });
});
