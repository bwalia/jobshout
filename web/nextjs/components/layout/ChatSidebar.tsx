"use client";

import { useEffect, useMemo, useState } from "react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { Plus, Search, Pencil, Trash2, ChevronLeft } from "lucide-react";
import { cn } from "@/lib/utils/cn";
import { sessionTitle, type ChatSession } from "@/lib/types/chat";
import {
  useChatSessions,
  useCreateChatSession,
  useDeleteChatSession,
} from "@/lib/hooks/useChat";
import { useUiStore } from "@/lib/store/ui-store";
import { PanelMenu } from "./PanelMenu";
import { SidebarFooter } from "./SidebarFooter";

function groupByRecency(sessions: ChatSession[]) {
  const now = Date.now();
  const day = 86400000;
  const groups: { label: string; items: ChatSession[] }[] = [
    { label: "Today", items: [] },
    { label: "Yesterday", items: [] },
    { label: "Previous 7 days", items: [] },
    { label: "Older", items: [] },
  ];
  for (const s of sessions) {
    const t = Date.parse(s.updated_at);
    const age = now - (Number.isNaN(t) ? now : t);
    if (age < day) groups[0].items.push(s);
    else if (age < 2 * day) groups[1].items.push(s);
    else if (age < 7 * day) groups[2].items.push(s);
    else groups[3].items.push(s);
  }
  return groups.filter((g) => g.items.length > 0);
}

function SidebarBody({
  collapsed,
  onToggleCollapse,
}: {
  collapsed: boolean;
  onToggleCollapse?: () => void;
}) {
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const activeSession = searchParams.get("session");
  const { chatTitleOverrides, setChatTitle } = useUiStore();

  const sessionsQuery = useChatSessions();
  const createSession = useCreateChatSession();
  const deleteSession = useDeleteChatSession();
  const sessions = useMemo(
    () => sessionsQuery.data?.data ?? [],
    [sessionsQuery.data]
  );
  const [query, setQuery] = useState("");
  const [renamingId, setRenamingId] = useState<string | null>(null);
  const [renameDraft, setRenameDraft] = useState("");

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return sessions;
    return sessions.filter((s) => {
      const title = (chatTitleOverrides[s.id] ?? sessionTitle(s)).toLowerCase();
      return title.includes(q);
    });
  }, [sessions, query, chatTitleOverrides]);

  const groups = useMemo(() => groupByRecency(filtered), [filtered]);

  function goSession(id: string) {
    router.push(`/chat?session=${id}`);
  }

  async function onNewChat() {
    const s = await createSession.mutateAsync();
    router.push(`/chat?session=${s.id}`);
  }

  function startRename(s: ChatSession) {
    setRenamingId(s.id);
    setRenameDraft(chatTitleOverrides[s.id] ?? sessionTitle(s));
  }

  function commitRename() {
    if (renamingId && renameDraft.trim()) {
      setChatTitle(renamingId, renameDraft.trim());
    }
    setRenamingId(null);
  }

  return (
    <aside
      className={cn(
        "flex h-full flex-col border-r border-sidebar-border bg-sidebar",
        collapsed ? "w-[72px]" : "w-[260px]"
      )}
    >
      <div
        className={cn(
          "flex h-14 shrink-0 items-center gap-2 border-b border-sidebar-border px-3",
          collapsed && "justify-center px-2"
        )}
      >
        <PanelMenu collapsed={collapsed} />
        {!collapsed && (
          <span className="flex-1 truncate text-sm font-semibold tracking-tight text-foreground">
            JobShout
          </span>
        )}
        {onToggleCollapse && (
          <button
            type="button"
            onClick={onToggleCollapse}
            className={cn(
              "hidden h-8 w-8 items-center justify-center rounded-md text-sidebar-foreground hover:bg-sidebar-muted hover:text-foreground lg:flex",
              collapsed && "rotate-180"
            )}
            aria-label={collapsed ? "Expand sidebar" : "Collapse sidebar"}
          >
            <ChevronLeft className="h-4 w-4" />
          </button>
        )}
      </div>

      <div className={cn("px-3 pt-3", collapsed && "px-2")}>
        <button
          type="button"
          onClick={() => void onNewChat()}
          disabled={createSession.isPending}
          title="New chat"
          className={cn(
            "flex w-full items-center justify-center gap-2 rounded-lg bg-primary px-3 py-2 text-sm font-medium text-primary-foreground transition-opacity hover:opacity-90 disabled:opacity-50",
            collapsed && "h-10 w-10 px-0"
          )}
        >
          <Plus className="h-4 w-4" />
          {!collapsed && "New chat"}
        </button>
      </div>

      {!collapsed && (
        <>
          <div className="px-3 pt-3">
            <div className="relative">
              <Search className="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
              <input
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder="Search chats"
                className="h-8 w-full rounded-md border border-sidebar-border bg-background pl-8 pr-2 text-sm outline-none placeholder:text-muted-foreground focus:border-ring"
              />
            </div>
          </div>
          <nav className="mt-2 flex-1 overflow-y-auto scrollbar-thin px-2 pb-2">
            {groups.length === 0 ? (
              <p className="px-2 py-6 text-center text-xs text-muted-foreground">
                No chats yet
              </p>
            ) : (
              groups.map((g) => (
                <div key={g.label} className="mb-3">
                  <p className="mb-1 px-2 text-[11px] font-medium text-muted-foreground">
                    {g.label}
                  </p>
                  <ul className="space-y-0.5">
                    {g.items.map((s) => {
                      const active =
                        pathname.startsWith("/chat") && activeSession === s.id;
                      const title =
                        chatTitleOverrides[s.id] ?? sessionTitle(s);
                      return (
                        <li key={s.id} className="group relative">
                          {renamingId === s.id ? (
                            <input
                              autoFocus
                              value={renameDraft}
                              onChange={(e) => setRenameDraft(e.target.value)}
                              onBlur={commitRename}
                              onKeyDown={(e) => {
                                if (e.key === "Enter") commitRename();
                                if (e.key === "Escape") setRenamingId(null);
                              }}
                              className="w-full rounded-md border border-ring bg-background px-2 py-1.5 text-sm outline-none"
                            />
                          ) : (
                            <button
                              type="button"
                              onClick={() => goSession(s.id)}
                              className={cn(
                                "w-full truncate rounded-md px-2 py-1.5 pr-14 text-left text-sm transition-colors",
                                active
                                  ? "bg-sidebar-muted text-foreground"
                                  : "text-sidebar-foreground hover:bg-sidebar-muted/70 hover:text-foreground"
                              )}
                            >
                              {title}
                            </button>
                          )}
                          {renamingId !== s.id && (
                            <div className="absolute right-1 top-1/2 hidden -translate-y-1/2 items-center gap-0.5 group-hover:flex">
                              <button
                                type="button"
                                aria-label="Rename"
                                onClick={(e) => {
                                  e.stopPropagation();
                                  startRename(s);
                                }}
                                className="rounded p-1 text-muted-foreground hover:bg-background hover:text-foreground"
                              >
                                <Pencil className="h-3 w-3" />
                              </button>
                              <button
                                type="button"
                                aria-label="Delete"
                                onClick={(e) => {
                                  e.stopPropagation();
                                  if (
                                    confirm("Delete this chat permanently?")
                                  ) {
                                    deleteSession.mutate(s.id, {
                                      onSuccess: () => {
                                        if (activeSession === s.id) {
                                          router.push("/chat");
                                        }
                                      },
                                    });
                                  }
                                }}
                                className="rounded p-1 text-muted-foreground hover:bg-background hover:text-destructive"
                              >
                                <Trash2 className="h-3 w-3" />
                              </button>
                            </div>
                          )}
                        </li>
                      );
                    })}
                  </ul>
                </div>
              ))
            )}
          </nav>
        </>
      )}

      {collapsed && <div className="flex-1" />}

      <SidebarFooter collapsed={collapsed} />
    </aside>
  );
}

export function ChatSidebar() {
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const {
    sidebarCollapsed,
    toggleSidebar,
    mobileSidebarOpen,
    setMobileSidebarOpen,
  } = useUiStore();

  useEffect(() => {
    setMobileSidebarOpen(false);
  }, [pathname, searchParams, setMobileSidebarOpen]);

  return (
    <>
      <div className="fixed left-0 top-0 z-30 hidden h-screen lg:block">
        <SidebarBody
          collapsed={sidebarCollapsed}
          onToggleCollapse={toggleSidebar}
        />
      </div>

      {mobileSidebarOpen && (
        <div className="fixed inset-0 z-40 lg:hidden">
          <button
            type="button"
            className="absolute inset-0 bg-black/40"
            aria-label="Close sidebar"
            onClick={() => setMobileSidebarOpen(false)}
          />
          <div className="absolute left-0 top-0 h-full shadow-card-hover">
            <SidebarBody collapsed={false} />
          </div>
        </div>
      )}
    </>
  );
}
