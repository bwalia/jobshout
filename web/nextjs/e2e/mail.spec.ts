import { test, expect } from "@playwright/test";
import { registerViaAPI, loginViaUI, navigateTo } from "./helpers";

const API_URL = process.env.E2E_API_URL ?? "http://localhost:8090/api/v1";

let creds: { email: string; password: string; token: string };

async function authJSON(path: string, init: RequestInit = {}) {
  const res = await fetch(`${API_URL}${path}`, {
    ...init,
    headers: {
      Authorization: `Bearer ${creds.token}`,
      "Content-Type": "application/json",
      ...(init.headers ?? {}),
    },
  });
  const text = await res.text();
  let body: unknown = null;
  if (text) {
    try {
      body = JSON.parse(text);
    } catch {
      body = text;
    }
  }
  return { res, body };
}

test.describe("Mail Agent", () => {
  test.beforeAll(async () => {
    creds = await registerViaAPI("mail");
  });

  test("a new org gets the built-in Mail Agent", async () => {
    const { res, body } = await authJSON("/agents?per_page=100");
    expect(res.status).toBe(200);
    const data = (body as { data: Array<{ name: string; status: string; metadata?: Record<string, string> }> }).data;
    const mail = data.find((a) => a.metadata?.builtin === "mail");
    expect(mail).toBeTruthy();
    expect(mail!.name).toBe("Mail Agent");
    expect(mail!.status).toBe("active");
  });

  test("connection is disconnected until Gmail OAuth completes", async () => {
    const { res, body } = await authJSON("/mail/connection");
    expect(res.status).toBe(200);
    const st = body as { connected: boolean; configured: boolean; email?: string };
    expect(st.connected).toBe(false);
    expect(st.email ?? "").toBe("");
  });

  test("sync without a connected mailbox is rejected and does not send", async () => {
    const { res, body } = await authJSON("/mail/sync", { method: "POST" });
    expect([409, 503]).toContain(res.status);
    expect(JSON.stringify(body)).not.toMatch(/refresh_token|ya29\./);
  });

  test("oauth start either 503s when unconfigured or returns a Google URL with no secrets", async () => {
    const { res, body } = await authJSON("/mail/connection/oauth/start", {
      method: "POST",
    });
    if (res.status === 503) {
      expect(JSON.stringify(body)).toMatch(/not configured|GMAIL_/i);
      return;
    }
    expect(res.status).toBe(200);
    const url = (body as { authorization_url: string }).authorization_url;
    expect(url).toContain("accounts.google.com");
    expect(url).not.toContain("client_secret");
    expect(url).not.toMatch(/GMAIL_TOKEN_KEY|refresh_token=/);
  });

  test("oauth callback without code redirects to the mail page", async () => {
    const apiOrigin = API_URL.replace(/\/api\/v1\/?$/, "");
    const res = await fetch(
      `${apiOrigin}/api/v1/mail/connection/oauth/callback`,
      { redirect: "manual" },
    );
    expect(res.status).toBe(302);
    const loc = res.headers.get("location") ?? "";
    expect(loc).toContain("/panel/task-manager");
    expect(loc).toContain("agent=mail");
    expect(loc).toContain("error=");
    expect(loc).not.toMatch(/refresh_token|client_secret/);
  });

  test("threads and drafts are empty before any sync", async () => {
    const threads = await authJSON("/mail/threads");
    expect(threads.res.status).toBe(200);
    expect(
      (threads.body as { data: unknown[] }).data ?? [],
    ).toEqual([]);

    const drafts = await authJSON("/mail/drafts");
    expect(drafts.res.status).toBe(200);
    expect((drafts.body as { data: unknown[] }).data ?? []).toEqual([]);
  });

  test("watch rules can be saved while disconnected", async () => {
    const { res, body } = await authJSON("/mail/connection", {
      method: "PATCH",
      body: JSON.stringify({
        rules: {
          senders: ["ops@example.com"],
          labels: [],
          subject_prefixes: ["[support]"],
        },
      }),
    });
    expect(res.status).toBe(200);
    const st = body as {
      connected: boolean;
      rules: { senders: string[]; subject_prefixes: string[] };
    };
    expect(st.connected).toBe(false);
    expect(st.rules.senders).toContain("ops@example.com");
    expect(st.rules.subject_prefixes).toContain("[support]");
  });

  test("mail page shows connect (or unconfigured) and never a send button", async ({
    page,
  }) => {
    await loginViaUI(page, creds.email, creds.password);
    await navigateTo(page, "/agents/mail");
    await expect(page.getByRole("heading", { name: "Mail Agent" })).toBeVisible();
    await expect(page.getByText("Draft only.")).toBeVisible();
    await expect(page.getByText("Approve (sends) or Reject")).toBeVisible();

    const connect = page.getByRole("button", { name: "Connect Gmail" });
    const unconfigured = page.getByText("Gmail OAuth is not configured");
    await expect(connect.or(unconfigured).first()).toBeVisible({
      timeout: 10_000,
    });
    await expect(
      page.getByRole("button", { name: "Approve and send" }),
    ).toHaveCount(0);
  });
});
