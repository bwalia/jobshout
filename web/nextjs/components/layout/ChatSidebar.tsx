"use client";

import { useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import {
  Plus,
  ListPlus,
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
import { panelFromPath, rememberPanelTransition } from "@/lib/panels";
import {
  NAV_BOTTOM,
  NAV_HOME,
  NAV_SECTIONS,
  NAV_SESSIONS,
  NEW_TASK_HREF,
  activeNavItem,
  type NavItem,
} from "@/lib/nav";
import { useNavBadges } from "@/lib/hooks/useNavBadges";
import { useLocalState } from "@/lib/utils/local-state";

// Every item must fit on a 768px-tall screen without scrolling, so rows are
// 32px; the active state stays unmistakable (filled row plus a primary bar)
// so nobody has to hunt for where they are.
function navItemClass(active: boolean, collapsed: boolean) {
  return cn(
    "flex w-full items-center gap-3 rounded-lg px-3 py-1.5 text-sm leading-5 transition-colors",
    "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1",
    collapsed && "h-10 w-10 justify-center px-0",
    active
      ? "bg-primary/12 font-semibold text-foreground shadow-[inset_3px_0_0_0_hsl(var(--primary))]"
      : "font-medium text-sidebar-foreground hover:bg-sidebar-muted hover:text-foreground"
  );
}

/** How many chats the sidebar lists before "View all". */
const RECENT_CHATS = 5;

function NavLink({
  item,
  active,
  collapsed,
  badge,
  onNavigate,
}: {
  item: NavItem;
  active: boolean;
  collapsed: boolean;
  badge?: number;
  onNavigate: (href: string) => void;
}) {
  const Icon = item.icon;
  return (
    <Link
      href={item.href}
      title={collapsed ? item.label : undefined}
      aria-current={active ? "page" : undefined}
      onClick={() => onNavigate(item.href)}
      className={cn(navItemClass(active, collapsed), "relative")}
    >
      <Icon className="h-4 w-4 shrink-0" />
      {!collapsed && <span className="min-w-0 flex-1 truncate">{item.label}</span>}
      {badge ? (
        <span
          className={cn(
            "inline-flex min-w-5 items-center justify-center rounded-full bg-destructive px-1.5 text-2xs font-bold text-destructive-foreground",
            collapsed && "absolute -right-1 -top-1"
          )}
          aria-label={`${badge} failed`}
        >
          {badge > 99 ? "99+" : badge}
        </span>
      ) : null}
    </Link>
  );
}

function SectionHeader({
  label,
  open,
  onToggle,
  controls,
}: {
  label: string;
  open: boolean;
  onToggle: () => void;
  controls: string;
}) {
  return (
    <button
      type="button"
      onClick={onToggle}
      aria-expanded={open}
      aria-controls={controls}
      className="mt-1.5 flex w-full items-center gap-1 rounded-md px-3 py-0.5 text-[11px] font-semibold uppercase tracking-wide text-muted-foreground hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
    >
      <span className="flex-1 text-left">{label}</span>
      <ChevronDown className={cn("h-3.5 w-3.5 transition-transform", !open && "-rotate-90")} />
    </button>
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
  const { chatTitleOverrides, setChatTitle, setCommandPaletteOpen } = useUiStore();
  const badges = useNavBadges();

  // Which sections are folded away, remembered per browser.
  const [closed, setClosed] = useLocalState<Record<string, boolean>>("jobshout-nav-sections", {});
  const [allChats, setAllChats] = useState(false);
  const toggle = (id: string) => setClosed((c) => ({ ...c, [id]: !c[id] }));

  const sessionsQuery = useChatSessions();
  const deleteSession = useDeleteChatSession();
  const sessions = useMemo(() => sessionsQuery.data?.data ?? [], [sessionsQuery.data]);
  const shownChats = allChats ? sessions : sessions.slice(0, RECENT_CHATS);
  const [renamingId, setRenamingId] = useState<string | null>(null);
  const [renameDraft, setRenameDraft] = useState("");

  const activeItem = activeNavItem(pathname);
  const activePanel = panelFromPath(pathname);
  const onEmptyChat = pathname.startsWith("/chat") && !activeSession;

  function markPanelNav(href: string) {
    rememberPanelTransition(activePanel, panelFromPath(href));
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

  const link = (item: NavItem) => (
    <NavLink
      key={item.id}
      item={item}
      active={activeItem?.id === item.id}
      collapsed={collapsed}
      badge={item.badge ? badges[item.badge] : undefined}
      onNavigate={markPanelNav}
    />
  );

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
        <Link
          href="/panel/dashboard"
          title="JobShout home"
          aria-label="JobShout home"
          className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg text-foreground hover:bg-sidebar-muted"
        >
          <Box className="h-6 w-6" />
        </Link>
        {!collapsed ? (
          <button
            type="button"
            onClick={() => setCommandPaletteOpen(true)}
            className="flex h-9 min-w-0 flex-1 items-center gap-2 rounded-lg border border-sidebar-border bg-background/60 px-2.5 text-sm text-muted-foreground hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            aria-label="Search pages, agents, projects and tasks"
          >
            <Search className="h-4 w-4 shrink-0" />
            <span className="flex-1 truncate text-left">Search</span>
            <kbd className="rounded border border-sidebar-border px-1 font-mono text-2xs">⌘K</kbd>
          </button>
        ) : (
          <button
            type="button"
            onClick={() => setCommandPaletteOpen(true)}
            title="Search (⌘K)"
            aria-label="Search"
            className="flex h-10 w-10 items-center justify-center rounded-md text-sidebar-foreground hover:bg-sidebar-muted hover:text-foreground"
          >
            <Search className="h-5 w-5" />
          </button>
        )}
        {onToggleCollapse && (
          <button
            type="button"
            onClick={onToggleCollapse}
            className={cn(
              "hidden h-8 w-8 shrink-0 items-center justify-center rounded-md text-sidebar-foreground hover:bg-sidebar-muted hover:text-foreground lg:flex",
              collapsed && "rotate-180"
            )}
            aria-label={collapsed ? "Expand sidebar" : "Collapse sidebar"}
          >
            <ChevronLeft className="h-4 w-4" />
          </button>
        )}
      </div>

      <div className={cn("flex shrink-0 gap-1.5 px-2 pt-2.5", collapsed && "flex-col items-center")}>
        <button
          type="button"
          onClick={() => router.push("/chat")}
          title="New chat"
          className={cn(
            "flex items-center justify-center gap-1.5 rounded-lg bg-primary text-sm font-semibold text-primary-foreground transition-opacity hover:opacity-90",
            "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1",
            collapsed ? "h-10 w-10" : "h-9 flex-1",
            onEmptyChat && "ring-2 ring-primary/30"
          )}
        >
          <Plus className="h-4 w-4" />
          {!collapsed && "New chat"}
        </button>
        <Link
          href={NEW_TASK_HREF}
          title="New task"
          onClick={() => markPanelNav(NEW_TASK_HREF)}
          className={cn(
            "flex items-center justify-center gap-1.5 rounded-lg border border-sidebar-border text-sm font-semibold text-foreground transition-colors hover:bg-sidebar-muted",
            "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1",
            collapsed ? "h-10 w-10" : "h-9 flex-1"
          )}
        >
          <ListPlus className="h-4 w-4" />
          {!collapsed && "New task"}
        </Link>
      </div>

      <nav
        aria-label="Main"
        className={cn(
          "mt-1.5 flex min-h-0 flex-1 flex-col gap-px overflow-y-auto px-2 pb-2 scrollbar-thin",
          collapsed && "items-center"
        )}
      >
        {link(NAV_HOME)}

        {NAV_SECTIONS.map((section) => {
          const open = collapsed || !closed[section.id];
          const listId = `nav-section-${section.id}`;
          return (
            <div key={section.id} className={cn("flex flex-col gap-0.5", collapsed && "items-center")}>
              {collapsed ? (
                <div className="my-1.5 h-px w-8 bg-sidebar-border" aria-hidden />
              ) : (
                <SectionHeader
                  label={section.label}
                  open={open}
                  onToggle={() => toggle(section.id)}
                  controls={listId}
                />
              )}
              {open && (
                <div id={listId} className={cn("flex flex-col gap-0.5", collapsed && "items-center")}>
                  {section.items.map(link)}
                </div>
              )}
            </div>
          );
        })}

        {!collapsed && (
          <div className="flex flex-col gap-0.5">
            <SectionHeader
              label="Chats"
              open={!closed.chats}
              onToggle={() => toggle("chats")}
              controls="nav-section-chats"
            />
            {!closed.chats && (
              <div id="nav-section-chats" className="flex flex-col gap-0.5">
                {sessions.length === 0 && (
                  <p className="px-3 py-1.5 text-sm text-muted-foreground">No chats yet</p>
                )}
                <ul className="space-y-0.5">
                  {shownChats.map((s) => {
                    const active = pathname.startsWith("/chat") && activeSession === s.id;
                    const title = chatTitleOverrides[s.id] ?? sessionTitle(s);
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
                            aria-label="Chat name"
                            className="w-full rounded-md border border-ring bg-background px-2 py-1.5 text-sm outline-none"
                          />
                        ) : (
                          <Link
                            href={`/chat?session=${s.id}`}
                            aria-current={active ? "page" : undefined}
                            className={cn(
                              "block w-full truncate rounded-md px-3 py-1.5 pr-14 text-sm font-medium transition-colors",
                              "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
                              active
                                ? "bg-sidebar-muted text-foreground"
                                : "text-sidebar-foreground hover:bg-sidebar-muted/70 hover:text-foreground"
                            )}
                          >
                            {title}
                          </Link>
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
                              aria-label={`Rename ${title}`}
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
                              aria-label={`Delete ${title}`}
                              onClick={(e) => {
                                e.stopPropagation();
                                if (confirm("Delete this chat permanently?")) {
                                  deleteSession.mutate(s.id, {
                                    onSuccess: () => {
                                      if (activeSession === s.id) router.push("/chat");
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
                {sessions.length > RECENT_CHATS && (
                  <button
                    type="button"
                    onClick={() => setAllChats((v) => !v)}
                    className="rounded-md px-3 py-1 text-left text-sm font-medium text-primary hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  >
                    {allChats ? "Show fewer" : `View all ${sessions.length} chats`}
                  </button>
                )}
                {link(NAV_SESSIONS)}
              </div>
            )}
          </div>
        )}
      </nav>

      <div className={cn("flex shrink-0 flex-col gap-px border-t border-sidebar-border px-2 py-1.5", collapsed && "items-center")}>
        {NAV_BOTTOM.map(link)}
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
