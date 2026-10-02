import { test, expect, type Page } from "@playwright/test";
import { registerViaAPI, loginViaUI } from "./helpers";

// The benchmark endpoints are answered from fixtures so the page's rendering
// is checked independently of what the backend has recorded. These are test
// fixtures, not benchmark results.
const RUN_ID = "11111111-2222-3333-4444-555555555555";

const models = [
  {
    provider: "gemini", model: "fixture-model", calls: 3, runs: 1, successes: 2, failures: 1, cancelled: 0,
    avg_duration_ms: 1500, min_duration_ms: 400, max_duration_ms: 3100, p50_duration_ms: 1000, p95_duration_ms: 2890,
    input_tokens: 300, output_tokens: 120, total_tokens: 420, calls_with_usage: 2,
    avg_input_tokens: 150, avg_output_tokens: 60, retries: 2, retried_calls: 1,
  },
];

const run = {
  run_kind: "course", run_id: RUN_ID, agent_name: "Course Generator", models: ["gemini/fixture-model"],
  call_count: 3, failures: 1, retries: 2, llm_time_ms: 4500, input_tokens: 300, output_tokens: 120,
  first_call_at: "2026-10-02T10:00:00Z", last_call_at: "2026-10-02T10:05:00Z",
  run_status: "completed", run_duration_ms: 310000,
};

const calls = [
  { id: "c1", run_kind: "course", run_id: RUN_ID, stage: "outline", attempt: 1, provider: "gemini", model: "fixture-model-001",
    model_reported: true, requested_model: "fixture-model", duration_ms: 1000, api_duration_ms: 950, api_attempt_count: 1, api_attempts: [], input_tokens: 150, output_tokens: 60, total_tokens: 210, reasoning_tokens: null,
    provider_request_id: "fixture-req-1", retries: 0, status: "success", started_at: null, completed_at: null },
  { id: "c2", run_kind: "course", run_id: RUN_ID, stage: "write", attempt: 1, provider: "gemini", model: "fixture-model",
    model_reported: false, requested_model: "fixture-model", duration_ms: 3100, api_duration_ms: 1200, api_attempt_count: 3, api_attempts: [], input_tokens: null, output_tokens: null, total_tokens: null, reasoning_tokens: null,
    provider_request_id: null, retries: 2, status: "success", started_at: null, completed_at: null },
  { id: "c3", run_kind: "course", run_id: RUN_ID, stage: "quiz", attempt: 2, provider: "gemini", model: "fixture-model",
    model_reported: false, requested_model: "fixture-model", duration_ms: 400, api_duration_ms: null, api_attempt_count: null, api_attempts: [], input_tokens: 150, output_tokens: 60, total_tokens: null, reasoning_tokens: null,
    provider_request_id: null, retries: 0, status: "failed", error: "gemini: overloaded", started_at: null, completed_at: null },
];

async function mockBenchmarks(page: Page) {
  await page.route("**/api/v1/benchmarks/models**", (r) => r.fulfill({ json: models }));
  await page.route("**/api/v1/benchmarks/runs?**", (r) =>
    r.fulfill({ json: { data: [run], total: 1, page: 1, per_page: 20, total_pages: 1 } }));
  await page.route(`**/api/v1/benchmarks/runs/course/${RUN_ID}`, (r) => r.fulfill({ json: { ...run, calls } }));
}

test.describe("LLM Benchmarks panel", () => {
  let creds: { email: string; password: string; token: string };

  test.beforeAll(async () => {
    creds = await registerViaAPI("bench");
  });

  test.beforeEach(async ({ page }) => {
    await mockBenchmarks(page);
    await loginViaUI(page, creds.email, creds.password);
  });

  test("shows provider, exact model, durations, tokens, retries and failures", async ({ page }) => {
    await page.goto("/panel/llm-benchmarks");
    await expect(page.locator("h1").first()).toContainText("LLM Benchmarks");

    const stats = page.getByTestId("llm-model-stats");
    await expect(stats).toContainText("gemini");
    await expect(stats).toContainText("fixture-model");
    for (const v of ["1.50 s", "1.00 s", "2.89 s", "400 ms", "3.10 s", "300", "120", "420"]) {
      await expect(stats).toContainText(v);
    }

    const runs = page.getByTestId("llm-runs");
    await expect(runs).toContainText("Course Generator");
    await expect(runs).toContainText("5m 10s"); // run duration, separate from call time
    await runs.getByText("Course Generator").click();

    const table = page.getByTestId("llm-calls-table");
    await expect(table).toContainText("outline");
    await expect(table).toContainText("attempt 2");
    await expect(table).toContainText("gemini: overloaded");
    // Unreported tokens are shown as unknown, never as 0.
    await expect(table.locator("tr").nth(2)).toContainText("—");
    // The reported model is shown, with the requested one beside it.
    await expect(table.locator("tr").nth(1)).toContainText("fixture-model-001");
    await expect(table.locator("tr").nth(1)).toContainText("requested fixture-model");
    await expect(table.locator("tr").nth(2)).toContainText("(not reported)");
  });
});
