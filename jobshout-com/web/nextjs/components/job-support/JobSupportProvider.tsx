"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";
import type { MatchItem } from "@/lib/job-support/events";
import type { SessionState } from "@/lib/job-support/types";

type Msg = {
  id: string;
  role: "user" | "assistant" | "status";
  content: string;
  matches?: MatchItem[];
};

type Ctx = {
  enabled: boolean | null;
  dockOpen: boolean;
  setDockOpen: (v: boolean) => void;
  busy: boolean;
  messages: Msg[];
  send: (text: string) => Promise<void>;
  reset: () => void;
};

const JobSupportContext = createContext<Ctx | null>(null);

const STORAGE_KEY = "jobshout-job-support-session";

function loadSession(): Partial<SessionState> | null {
  try {
    const raw = sessionStorage.getItem(STORAGE_KEY);
    return raw ? (JSON.parse(raw) as Partial<SessionState>) : null;
  } catch {
    return null;
  }
}

function saveSession(s: Partial<SessionState>) {
  try {
    sessionStorage.setItem(STORAGE_KEY, JSON.stringify(s));
  } catch {
    /* ignore */
  }
}

export function JobSupportProvider({ children }: { children: ReactNode }) {
  const [enabled, setEnabled] = useState<boolean | null>(null);
  const [dockOpen, setDockOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [messages, setMessages] = useState<Msg[]>([
    {
      id: "welcome",
      role: "assistant",
      content:
        "Hi — I'm Job Support. Are you hiring (share a job brief) or looking for work (paste a short CV)? Matching runs in our private NebulaDB; your contact details stay in our CRM.",
    },
  ]);
  const [session, setSession] = useState<Partial<SessionState>>({});

  useEffect(() => {
    const stored = loadSession();
    if (stored?.id) setSession(stored);
    fetch("/api/job-support/chat", { cache: "no-store" })
      .then((r) => r.json())
      .then((j: { enabled?: boolean }) => setEnabled(Boolean(j.enabled)))
      .catch(() => setEnabled(false));
  }, []);

  const reset = useCallback(() => {
    setSession({});
    try {
      sessionStorage.removeItem(STORAGE_KEY);
    } catch {
      /* ignore */
    }
    setMessages([
      {
        id: `welcome_${Date.now()}`,
        role: "assistant",
        content:
          "Fresh start. Hiring or looking for work? I'll ask for a brief or CV, then match privately.",
      },
    ]);
  }, []);

  const send = useCallback(
    async (text: string) => {
      const trimmed = text.trim();
      if (!trimmed || busy) return;
      setBusy(true);
      const userId = `u_${Date.now()}`;
      setMessages((m) => [...m, { id: userId, role: "user", content: trimmed }]);

      const history = messages
        .filter((m) => m.role === "user" || m.role === "assistant")
        .slice(-10)
        .map((m) => ({
          role: m.role as "user" | "assistant",
          content: m.content,
        }));

      try {
        const res = await fetch("/api/job-support/chat", {
          method: "POST",
          headers: { "content-type": "application/json" },
          body: JSON.stringify({ message: trimmed, session, history }),
        });
        if (!res.ok || !res.body) {
          const err = await res.json().catch(() => null);
          throw new Error(
            err?.error?.message ?? `Request failed (${res.status})`,
          );
        }

        const reader = res.body.getReader();
        const dec = new TextDecoder();
        let buf = "";
        let assistantId = `a_${Date.now()}`;
        let assistantText = "";
        let pendingMatches: MatchItem[] | undefined;

        const ensureAssistant = () => {
          setMessages((m) => {
            if (m.some((x) => x.id === assistantId)) return m;
            return [
              ...m,
              { id: assistantId, role: "assistant", content: "", matches: pendingMatches },
            ];
          });
        };

        while (true) {
          const { done, value } = await reader.read();
          if (done) break;
          buf += dec.decode(value, { stream: true });
          const parts = buf.split("\n\n");
          buf = parts.pop() ?? "";
          for (const part of parts) {
            const line = part
              .split("\n")
              .find((l) => l.startsWith("data: "));
            if (!line) continue;
            let ev: {
              type: string;
              text?: string;
              message?: string;
              items?: MatchItem[];
              state?: Partial<SessionState>;
            };
            try {
              ev = JSON.parse(line.slice(6));
            } catch {
              continue;
            }
            if (ev.type === "text" && ev.text) {
              assistantText += (assistantText ? "" : "") + ev.text;
              // Model returns full reply per turn — replace, don't append chunks.
              assistantText = ev.text;
              ensureAssistant();
              setMessages((m) =>
                m.map((x) =>
                  x.id === assistantId
                    ? { ...x, content: assistantText, matches: pendingMatches }
                    : x,
                ),
              );
            } else if (ev.type === "matches" && ev.items) {
              pendingMatches = ev.items;
              ensureAssistant();
              setMessages((m) =>
                m.map((x) =>
                  x.id === assistantId ? { ...x, matches: pendingMatches } : x,
                ),
              );
            } else if (ev.type === "status" && ev.message) {
              setMessages((m) => [
                ...m,
                {
                  id: `s_${Date.now()}_${Math.random().toString(36).slice(2, 6)}`,
                  role: "status",
                  content: ev.message!,
                },
              ]);
            } else if (ev.type === "session_state" && ev.state) {
              setSession(ev.state);
              saveSession(ev.state);
            } else if (ev.type === "error" && ev.message) {
              setMessages((m) => [
                ...m,
                {
                  id: `e_${Date.now()}`,
                  role: "assistant",
                  content: ev.message!,
                },
              ]);
            }
          }
        }
      } catch (err) {
        setMessages((m) => [
          ...m,
          {
            id: `e_${Date.now()}`,
            role: "assistant",
            content: err instanceof Error ? err.message : "Something went wrong.",
          },
        ]);
      } finally {
        setBusy(false);
      }
    },
    [busy, messages, session],
  );

  const value = useMemo(
    () => ({ enabled, dockOpen, setDockOpen, busy, messages, send, reset }),
    [enabled, dockOpen, busy, messages, send, reset],
  );

  return (
    <JobSupportContext.Provider value={value}>{children}</JobSupportContext.Provider>
  );
}

export function useJobSupport() {
  const ctx = useContext(JobSupportContext);
  if (!ctx) throw new Error("useJobSupport requires JobSupportProvider");
  return ctx;
}
