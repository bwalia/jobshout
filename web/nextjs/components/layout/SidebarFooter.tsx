"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { LogOut, Moon, Sun, User, ChevronUp } from "lucide-react";
import { useTheme } from "next-themes";
import { useAuthStore } from "@/lib/store/auth-store";
import { clearTokens } from "@/lib/auth/auth";
import { cn } from "@/lib/utils/cn";
import { useHealth } from "@/lib/hooks/useHealth";

function initials(name: string): string {
  return name
    .split(" ")
    .filter(Boolean)
    .slice(0, 2)
    .map((p) => p[0].toUpperCase())
    .join("");
}

export function SidebarFooter({ collapsed }: { collapsed: boolean }) {
  const router = useRouter();
  const { user, logout } = useAuthStore();
  const { resolvedTheme, setTheme } = useTheme();
  const [mounted, setMounted] = useState(false);
  const [menuOpen, setMenuOpen] = useState(false);
  const menuRef = useRef<HTMLDivElement>(null);

  useEffect(() => setMounted(true), []);

  useEffect(() => {
    function onDoc(e: MouseEvent) {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        setMenuOpen(false);
      }
    }
    document.addEventListener("mousedown", onDoc);
    return () => document.removeEventListener("mousedown", onDoc);
  }, []);

  const label = user?.full_name ?? "Account";
  const avatar = useMemo(
    () => (user?.full_name ? initials(user.full_name) : "?"),
    [user?.full_name]
  );
  const isDark = mounted && resolvedTheme === "dark";
  const health = useHealth();

  function handleLogout() {
    clearTokens();
    logout();
    router.replace("/login");
  }

  if (collapsed) {
    return (
      <div className="flex flex-col items-center gap-2 border-t border-sidebar-border px-2 py-3">
        <button
          type="button"
          onClick={() => setTheme(isDark ? "light" : "dark")}
          className="flex h-9 w-9 items-center justify-center rounded-lg text-sidebar-foreground hover:bg-sidebar-muted hover:text-foreground"
          aria-label={isDark ? "Switch to light" : "Switch to dark"}
        >
          {isDark ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
        </button>
        <div ref={menuRef} className="relative">
          <button
            type="button"
            onClick={() => setMenuOpen((v) => !v)}
            className="flex h-9 w-9 items-center justify-center rounded-lg bg-primary text-[11px] font-semibold text-primary-foreground"
            aria-label="Account menu"
            aria-expanded={menuOpen}
          >
            {avatar}
          </button>
          {menuOpen && (
            <AccountMenu
              onProfile={() => {
                setMenuOpen(false);
                router.push("/panel/settings");
              }}
              onLogout={handleLogout}
              align="left"
            />
          )}
        </div>
        <BuildStamp info={health.data} collapsed />
      </div>
    );
  }

  return (
    <div className="border-t border-sidebar-border px-3 py-3">
      <div className="flex items-center gap-2">
        <div ref={menuRef} className="relative min-w-0 flex-1">
          <button
            type="button"
            onClick={() => setMenuOpen((v) => !v)}
            className={cn(
              "flex w-full items-center gap-2.5 rounded-lg px-2 py-1.5 text-left transition-colors",
              "hover:bg-sidebar-muted",
              menuOpen && "bg-sidebar-muted"
            )}
            aria-haspopup="true"
            aria-expanded={menuOpen}
          >
            <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-primary text-[11px] font-semibold text-primary-foreground">
              {avatar}
            </span>
            <span className="min-w-0 flex-1">
              <span className="block truncate text-sm font-medium text-foreground">
                {label}
              </span>
              <span className="block truncate text-[11px] text-muted-foreground">
                {user?.email}
              </span>
            </span>
            <ChevronUp
              className={cn(
                "h-3.5 w-3.5 shrink-0 text-muted-foreground transition-transform",
                !menuOpen && "rotate-180"
              )}
            />
          </button>
          {menuOpen && (
            <AccountMenu
              onProfile={() => {
                setMenuOpen(false);
                router.push("/panel/settings");
              }}
              onLogout={handleLogout}
              align="left"
            />
          )}
        </div>
        <button
          type="button"
          onClick={() => setTheme(isDark ? "light" : "dark")}
          className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg text-sidebar-foreground hover:bg-sidebar-muted hover:text-foreground"
          aria-label={isDark ? "Switch to light" : "Switch to dark"}
        >
          {isDark ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
        </button>
      </div>
      <BuildStamp info={health.data} />
    </div>
  );
}

function AccountMenu({
  onProfile,
  onLogout,
  align,
}: {
  onProfile: () => void;
  onLogout: () => void;
  align: "left" | "right";
}) {
  return (
    <div
      role="menu"
      className={cn(
        "absolute bottom-full z-50 mb-2 w-52 rounded-lg border border-border bg-popover p-1 shadow-card-hover",
        align === "left" ? "left-0" : "right-0"
      )}
    >
      <button
        type="button"
        role="menuitem"
        onClick={onProfile}
        className="flex w-full items-center gap-2 rounded-md px-2.5 py-2 text-sm hover:bg-secondary"
      >
        <User className="h-4 w-4 text-muted-foreground" />
        Profile & workspace
      </button>
      <button
        type="button"
        role="menuitem"
        onClick={onLogout}
        className="flex w-full items-center gap-2 rounded-md px-2.5 py-2 text-sm text-destructive hover:bg-destructive/10"
      >
        <LogOut className="h-4 w-4" />
        Sign out
      </button>
    </div>
  );
}

function BuildStamp({
  info,
  collapsed,
}: {
  info?: { version: string; env?: string; deployed_at?: string };
  collapsed?: boolean;
}) {
  if (!info?.version) return null;
  const deployed = formatDeployed(info.deployed_at);
  const absolute = info.deployed_at
    ? new Date(info.deployed_at).toLocaleString()
    : undefined;
  const detail = [info.env, deployed ? `Deployed ${deployed}` : null]
    .filter(Boolean)
    .join(" · ");
  const title = [info.version, info.env, absolute && `Deployed ${absolute}`]
    .filter(Boolean)
    .join(" · ");

  if (collapsed) {
    return (
      <span
        className="max-w-full truncate px-0.5 text-center font-mono text-[9px] leading-tight text-muted-foreground"
        title={title}
      >
        {info.version}
      </span>
    );
  }

  return (
    <div
      className="mt-2 flex items-start justify-between gap-2 px-2"
      title={title}
    >
      <span className="min-w-0 truncate text-[10px] leading-tight text-muted-foreground">
        {detail || "\u00a0"}
      </span>
      <span className="shrink-0 font-mono text-[10px] leading-tight text-muted-foreground">
        {info.version}
      </span>
    </div>
  );
}

function formatDeployed(iso?: string): string | null {
  if (!iso) return null;
  const t = Date.parse(iso);
  if (Number.isNaN(t)) return null;
  const mins = Math.floor((Date.now() - t) / 60_000);
  if (mins < 1) return "just now";
  if (mins < 60) return `${mins}m ago`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  if (days < 30) return `${days}d ago`;
  return new Date(t).toLocaleDateString();
}
