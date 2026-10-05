"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { Search } from "lucide-react";
import { cn } from "@/lib/utils/cn";
import { ALL_NAV_ITEMS } from "@/lib/nav";
import { agentHref } from "@/lib/agents/links";
import { useAgents } from "@/lib/hooks/useAgents";
import { useProjects } from "@/lib/hooks/useProjects";
import { useAllTasks } from "@/lib/hooks/useTasks";
import { useChatSessions } from "@/lib/hooks/useChat";
import { sessionTitle } from "@/lib/types/chat";
import { useUiStore } from "@/lib/store/ui-store";

type Group = "Actions" | "Pages" | "Agents" | "Projects" | "Tasks" | "Chats";

type Item =
  | { kind: "link"; group: Group; id: string; label: string; hint?: string; keywords?: string; href: string }
  | { kind: "action"; group: Group; id: string; label: string; hint?: string; keywords?: string; run: () => void };

/** Most results shown per group, so one long list cannot bury the others. */
const PER_GROUP = 6;

export function CommandPalette() {
  const router = useRouter();
  const open = useUiStore((s) => s.commandPaletteOpen);
  const setOpen = useUiStore((s) => s.setCommandPaletteOpen);
  const toggle = useUiStore((s) => s.toggleCommandPalette);
  const [query, setQuery] = useState("");
  const [focusIdx, setFocusIdx] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);
  const itemRefs = useRef<(HTMLButtonElement | null)[]>([]);
  const sessionsQuery = useChatSessions();
  const overrides = useUiStore((s) => s.chatTitleOverrides);
  const { data: agentsResp } = useAgents({ per_page: 100 });
  const { data: projectsResp } = useProjects({ per_page: 100 });
  const { data: tasksResp } = useAllTasks();

  const items = useMemo<Item[]>(() => {
    const q = query.trim().toLowerCase();
    const all: Item[] = [
      { kind: "action", group: "Actions", id: "new-task", label: "New task", keywords: "create add", run: () => router.push("/panel/tasks?new=1") },
      { kind: "action", group: "Actions", id: "new-agent", label: "New agent", keywords: "create add", run: () => router.push("/panel/agents?new=1") },
      { kind: "action", group: "Actions", id: "new-chat", label: "New chat", keywords: "create ask", run: () => router.push("/chat") },
      { kind: "action", group: "Actions", id: "run-workflow", label: "Run a workflow", keywords: "automation start", run: () => router.push("/panel/workflows") },
      ...ALL_NAV_ITEMS.map((n): Item => ({
        kind: "link",
        group: "Pages",
        id: n.id,
        label: n.label,
        keywords: (n.keywords ?? []).join(" "),
        href: n.href,
      })),
      ...(agentsResp?.data ?? []).map((a): Item => ({
        kind: "link",
        group: "Agents",
        id: a.id,
        label: a.name,
        hint: a.role,
        keywords: `${a.role} ${a.model_name ?? ""}`,
        href: agentHref(a.id),
      })),
      ...(projectsResp?.data ?? []).map((p): Item => ({
        kind: "link",
        group: "Projects",
        id: p.id,
        label: p.name,
        href: `/panel/projects?project=${p.id}`,
      })),
      ...(sessionsQuery.data?.data ?? []).slice(0, 20).map((s): Item => ({
        kind: "link",
        group: "Chats",
        id: s.id,
        label: overrides[s.id] ?? sessionTitle(s),
        href: `/chat?session=${s.id}`,
      })),
    ];
    // Tasks are many; only search them once something is typed.
    if (q) {
      for (const t of tasksResp?.data ?? []) {
        all.push({ kind: "link", group: "Tasks", id: t.id, label: t.title, href: `/panel/tasks?task=${t.id}` });
      }
    }

    const matches = (i: Item) => !q || `${i.label} ${i.hint ?? ""} ${i.keywords ?? ""}`.toLowerCase().includes(q);
    const order: Group[] = q
      ? ["Pages", "Agents", "Projects", "Tasks", "Actions", "Chats"]
      : ["Actions", "Pages", "Agents", "Projects", "Chats"];
    const out: Item[] = [];
    for (const g of order) {
      const hits = all.filter((i) => i.group === g && matches(i));
      // With no query every page is listed, so the palette doubles as a map.
      out.push(...(!q && g === "Pages" ? hits : hits.slice(0, PER_GROUP)));
    }
    return out;
  }, [query, sessionsQuery.data, overrides, router, agentsResp, projectsResp, tasksResp]);

  const close = useCallback(() => {
    setOpen(false);
    setQuery("");
    setFocusIdx(0);
  }, [setOpen]);

  const activate = useCallback(
    (item: Item) => {
      if (item.kind === "action") item.run();
      else router.push(item.href);
      close();
    },
    [router, close]
  );

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        toggle();
      }
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "n") {
        const target = e.target as HTMLElement | null;
        if (target?.closest("input,textarea,[contenteditable]")) return;
        e.preventDefault();
        router.push("/chat");
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [router, toggle]);

  useEffect(() => {
    if (open) {
      setFocusIdx(0);
      requestAnimationFrame(() => inputRef.current?.focus());
    } else {
      setQuery("");
      setFocusIdx(0);
    }
  }, [open]);

  useEffect(() => {
    if (!open) return;
    document.body.style.overflow = "hidden";
    return () => {
      document.body.style.overflow = "";
    };
  }, [open]);

  useEffect(() => {
    if (!open) return;
    itemRefs.current[focusIdx]?.scrollIntoView({ block: "nearest" });
  }, [focusIdx, open]);

  useEffect(() => {
    if (!open) return;
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") {
        e.preventDefault();
        close();
      } else if (e.key === "ArrowDown") {
        e.preventDefault();
        setFocusIdx((i) => Math.min(i + 1, items.length - 1));
      } else if (e.key === "ArrowUp") {
        e.preventDefault();
        setFocusIdx((i) => Math.max(i - 1, 0));
      } else if (e.key === "Enter") {
        e.preventDefault();
        const item = items[focusIdx];
        if (item) activate(item);
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, items, focusIdx, close, activate]);

  if (!open) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-start justify-center pt-[15vh]">
      <button
        type="button"
        className="absolute inset-0 bg-black/40"
        aria-label="Close command palette"
        onClick={close}
      />
      <div
        role="dialog"
        aria-label="Command palette"
        className="relative z-10 w-full max-w-lg overflow-hidden rounded-xl border border-border bg-popover shadow-card-hover"
      >
        <div className="flex items-center gap-2 border-b border-border px-3">
          <Search className="h-4 w-4 text-muted-foreground" />
          <input
            ref={inputRef}
            value={query}
            onChange={(e) => {
              setQuery(e.target.value);
              setFocusIdx(0);
            }}
            placeholder="Search pages, agents, projects, tasks…"
            role="combobox"
            aria-expanded="true"
            aria-controls="command-palette-list"
            aria-activedescendant={items[focusIdx] ? `command-item-${focusIdx}` : undefined}
            aria-label="Search"
            className="h-11 flex-1 bg-transparent text-sm outline-none placeholder:text-muted-foreground"
          />
          <kbd className="hidden rounded border border-border px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground sm:inline">
            esc
          </kbd>
        </div>
        <ul id="command-palette-list" role="listbox" aria-label="Results" className="max-h-[60vh] overflow-y-auto scrollbar-thin p-1">
          {items.length === 0 ? (
            <li className="px-3 py-6 text-center text-sm text-muted-foreground">
              Nothing matches “{query}”.
            </li>
          ) : (
            items.map((item, idx) => (
              <li key={`${item.group}-${item.id}`} role="presentation">
                {(idx === 0 || items[idx - 1].group !== item.group) && (
                  <p className="px-3 pb-1 pt-2 text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">
                    {item.group}
                  </p>
                )}
                <button
                  type="button"
                  id={`command-item-${idx}`}
                  role="option"
                  aria-selected={idx === focusIdx}
                  ref={(el) => {
                    itemRefs.current[idx] = el;
                  }}
                  onMouseEnter={() => setFocusIdx(idx)}
                  onClick={() => activate(item)}
                  className={cn(
                    "flex w-full items-center justify-between gap-3 rounded-md px-3 py-2 text-left text-sm",
                    idx === focusIdx ? "bg-accent text-accent-foreground" : "hover:bg-secondary"
                  )}
                >
                  <span className="truncate">{item.label}</span>
                  {item.hint && <span className="truncate text-[11px] text-muted-foreground">{item.hint}</span>}
                </button>
              </li>
            ))
          )}
        </ul>
      </div>
    </div>
  );
}
