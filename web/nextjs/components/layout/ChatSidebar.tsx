"use client";

import { useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import {
  Plus,
  Search,
  Pencil,
  Trash2,
  ChevronLeft,
  ChevronDown,
  Box,
} from "lucide-react";
import { cn } from "@/lib/utils/cn";
import { sessionTitle, type ChatSession } from "@/lib/types/chat";
import {
  useChatSessions,
  useDeleteChatSession,
} from "@/lib/hooks/useChat";
import { useUiStore } from "@/lib/store/ui-store";
import { SidebarFooter } from "./SidebarFooter";
import {
  APP_NAV_PANELS,
  SIDEBAR_PRIMARY,
  isAppNavPath,
  panelFromPath,
  rememberPanelTransition,
} from "@/lib/panels";

function startOfLocalDay(d: Date): number {
  const x = new Date(d);
  x.setHours(0, 0, 0, 0);
  return x.getTime();
}

function groupByRecency(sessions: ChatSession[]) {
  const today = startOfLocalDay(new Date());
  const yesterday = today - 86400000;
  const week = today - 6 * 86400000;
  const groups: { label: string; items: ChatSession[] }[] = [
    { label: "Today", items: [] },
    { label: "Yesterday", items: [] },
    { label: "Last 7 days", items: [] },
    { label: "Older", items: [] },
  ];
  for (const s of sessions) {
    const t = Date.parse(s.updated_at);
    const day = Number.isNaN(t) ? today : startOfLocalDay(new Date(t));
    if (day >= today) groups[0].items.push(s);
    else if (day >= yesterday) groups[1].items.push(s);
    else if (day >= week) groups[2].items.push(s);
    else groups[3].items.push(s);
  }
  return groups.filter((g) => g.items.length > 0);
}

function navItemClass(active: boolean, collapsed: boolean) {
  return cn(
    "flex w-full items-center gap-2.5 rounded-lg px-2.5 py-2 text-sm transition-colors",
    collapsed && "h-10 w-10 justify-center px-0",
    active
      ? "bg-sidebar-muted text-foreground"
      : "text-sidebar-foreground hover:bg-sidebar-muted/70 hover:text-foreground"
  );
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
  const { chatTitleOverrides, setChatTitle, setCommandPaletteOpen, workspaceNavCollapsed, setWorkspaceNavCollapsed } =
    useUiStore();

  const sessionsQuery = useChatSessions();
  const deleteSession = useDeleteChatSession();
  const sessions = useMemo(
    () => sessionsQuery.data?.data ?? [],
    [sessionsQuery.data]
  );
  const [renamingId, setRenamingId] = useState<string | null>(null);
  const [renameDraft, setRenameDraft] = useState("");

  const groups = useMemo(() => groupByRecency(sessions), [sessions]);
  const activePanel = panelFromPath(pathname);
  const onDashboard = pathname.startsWith("/panel/dashboard");
  const showAppNav = isAppNavPath(pathname) && !workspaceNavCollapsed;
  const onEmptyChat = pathname.startsWith("/chat") && !activeSession;

  function goSession(id: string) {
    router.push(`/chat?session=${id}`);
  }

  function onNewChat() {
    router.push("/chat");
  }

  function markPanelNav(href: string) {
    rememberPanelTransition(activePanel, panelFromPath(href));
  }

  function onDashboardClick(e: React.MouseEvent<HTMLAnchorElement>) {
    if (onDashboard) {
      e.preventDefault();
      setWorkspaceNavCollapsed(!workspaceNavCollapsed);
      return;
    }
    setWorkspaceNavCollapsed(false);
    markPanelNav("/panel/dashboard");
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
          "flex shrink-0 items-center gap-1 border-b border-sidebar-border px-3",
          collapsed ? "h-auto flex-col py-3" : "h-14"
        )}
      >
        <button
          type="button"
          onClick={onNewChat}
          title="JobShout"
          aria-label="JobShout home"
          className="flex h-8 w-8 items-center justify-center rounded-lg text-foreground hover:bg-sidebar-muted"
        >
          <Box className="h-5 w-5" />
        </button>
        {!collapsed && <div className="flex-1" />}
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
        <button
          type="button"
          onClick={() => setCommandPaletteOpen(true)}
          title="Search"
          aria-label="Search"
          className="flex h-8 w-8 items-center justify-center rounded-md text-sidebar-foreground hover:bg-sidebar-muted hover:text-foreground"
        >
          <Search className="h-4 w-4" />
        </button>
      </div>

      <div className={cn("flex flex-col gap-0.5 px-2 pt-3", collapsed && "items-center px-2")}>
        <button
          type="button"
          onClick={onNewChat}
          title="New chat"
          aria-label="New chat"
          className={navItemClass(onEmptyChat, collapsed)}
        >
          <Plus className="h-4 w-4 shrink-0" />
          {!collapsed && "New Chat"}
        </button>

        {SIDEBAR_PRIMARY.map((item) => {
          const Icon = item.icon;
          const isDashboard = item.id === "dashboard";
          const active = isDashboard
            ? onDashboard
            : pathname.startsWith(item.href);
          return (
            <Link
              key={item.id}
              href={item.href}
              title={item.label}
              aria-current={active ? "page" : undefined}
              aria-expanded={isDashboard ? showAppNav : undefined}
              onClick={isDashboard ? onDashboardClick : () => markPanelNav(item.href)}
              className={navItemClass(active, collapsed)}
            >
              <Icon className="h-4 w-4 shrink-0" />
              {!collapsed && (
                <>
                  <span className="min-w-0 flex-1 truncate">{item.label}</span>
                  {isDashboard && isAppNavPath(pathname) && (
                    <ChevronDown
                      className={cn(
                        "h-3.5 w-3.5 shrink-0 text-muted-foreground transition-transform",
                        showAppNav && "rotate-180"
                      )}
                    />
                  )}
                </>
              )}
            </Link>
          );
        })}
      </div>

      <div
        className={cn(
          "mt-1 flex min-h-0 flex-1 flex-col overflow-y-auto scrollbar-thin",
          collapsed && "items-center"
        )}
      >
        {showAppNav && (
        <nav
          className={cn(
            "flex flex-col gap-0.5 border-t border-sidebar-border px-2 pt-2",
            collapsed && "items-center",
            !collapsed && "ml-2"
          )}
          aria-label="Workspace"
        >
          {APP_NAV_PANELS.map((panel) => {
            const Icon = panel.icon;
            const active = panel.id === activePanel;
            return (
              <Link
                key={panel.id}
                href={panel.href}
                title={panel.label}
                aria-current={active ? "page" : undefined}
                onClick={() => {
                setWorkspaceNavCollapsed(false);
                markPanelNav(panel.href);
              }}
                className={navItemClass(active, collapsed)}
              >
                <Icon className="h-4 w-4 shrink-0" />
                {!collapsed && <span className="truncate">{panel.label}</span>}
              </Link>
            );
          })}
        </nav>
      )}

      {!collapsed && (
        <nav className="mt-2 px-2 pb-2">
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
                          <div
                            className={cn(
                              "absolute right-1 top-1/2 flex -translate-y-1/2 items-center gap-0.5",
                              !active &&
                                "opacity-0 group-hover:opacity-100 group-focus-within:opacity-100 focus-within:opacity-100"
                            )}
                          >
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
                                if (confirm("Delete this chat permanently?")) {
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
      )}
      </div>

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
