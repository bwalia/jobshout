"use client";

import { useState, useRef, useEffect } from "react";
import { useRouter } from "next/navigation";
import { LogOut, ChevronDown, User, Menu, Bell } from "lucide-react";
import { useAuthStore } from "@/lib/store/auth-store";
import { clearTokens } from "@/lib/auth/auth";
import { cn } from "@/lib/utils/cn";
import { ThemeToggle } from "./ThemeToggle";

function getInitials(fullName: string): string {
  return fullName
    .split(" ")
    .filter(Boolean)
    .slice(0, 2)
    .map((part) => part[0].toUpperCase())
    .join("");
}

interface TopbarProps {
  onMenuToggle?: () => void;
}

export function Topbar({ onMenuToggle }: TopbarProps) {
  const router = useRouter();
  const { user, logout } = useAuthStore();
  const [menuOpen, setMenuOpen] = useState(false);
  const menuRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    function handleClickOutside(event: MouseEvent) {
      if (menuRef.current && !menuRef.current.contains(event.target as Node)) {
        setMenuOpen(false);
      }
    }
    document.addEventListener("mousedown", handleClickOutside);
    return () => document.removeEventListener("mousedown", handleClickOutside);
  }, []);

  function handleLogout() {
    clearTokens();
    logout();
    router.replace("/login");
  }

  const initials = user?.full_name ? getInitials(user.full_name) : "?";

  return (
    <header className="sticky top-0 z-20 flex h-16 shrink-0 items-center justify-between border-b border-border bg-card px-6">
      {/* Left: mobile menu + breadcrumb area */}
      <div className="flex items-center gap-3">
        {onMenuToggle && (
          <button
            onClick={onMenuToggle}
            className="flex h-9 w-9 items-center justify-center rounded-lg text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground lg:hidden"
            aria-label="Toggle menu"
          >
            <Menu className="h-5 w-5" />
          </button>
        )}
        <span className="text-sm font-semibold text-foreground lg:hidden">
          Jobshout
        </span>
      </div>

      {/* Right: controls */}
      <div className="ml-auto flex items-center gap-2">
        <ThemeToggle />

        {/* Notification bell placeholder */}
        <button
          type="button"
          className="flex h-9 w-9 items-center justify-center rounded-lg text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground"
          aria-label="Notifications"
        >
          <Bell className="h-4 w-4" />
        </button>

        {/* Divider */}
        <div className="mx-1 h-6 w-px bg-border" />

        {/* User menu */}
        <div ref={menuRef} className="relative">
          <button
            type="button"
            onClick={() => setMenuOpen((prev) => !prev)}
            className={cn(
              "flex items-center gap-2.5 rounded-lg px-2.5 py-1.5 text-sm transition-colors",
              "hover:bg-secondary",
              menuOpen && "bg-secondary"
            )}
            aria-haspopup="true"
            aria-expanded={menuOpen}
          >
            <span className="flex h-8 w-8 items-center justify-center rounded-lg bg-primary text-xs font-semibold text-primary-foreground">
              {initials}
            </span>
            <div className="hidden text-left sm:block">
              <p className="max-w-[120px] truncate text-sm font-medium text-foreground">
                {user?.full_name ?? "Account"}
              </p>
              <p className="max-w-[120px] truncate text-[11px] text-muted-foreground">
                {user?.email}
              </p>
            </div>
            <ChevronDown
              className={cn(
                "hidden h-3.5 w-3.5 text-muted-foreground transition-transform sm:block",
                menuOpen && "rotate-180"
              )}
            />
          </button>

          {menuOpen && (
            <div
              role="menu"
              className={cn(
                "absolute right-0 top-full z-50 mt-2 w-56 origin-top-right",
                "rounded-xl border border-border bg-card p-1.5 shadow-lg",
                "animate-in fade-in-0 zoom-in-95"
              )}
            >
              <div className="flex items-center gap-3 rounded-lg px-3 py-2.5">
                <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-primary text-xs font-semibold text-primary-foreground">
                  {initials}
                </span>
                <div className="min-w-0">
                  {user?.full_name && (
                    <p className="truncate text-sm font-medium text-foreground">
                      {user.full_name}
                    </p>
                  )}
                  <p className="truncate text-xs text-muted-foreground">
                    {user?.email}
                  </p>
                </div>
              </div>

              <div className="my-1 h-px bg-border" />

              <button
                type="button"
                role="menuitem"
                onClick={() => {
                  setMenuOpen(false);
                  router.push("/settings");
                }}
                className="flex w-full items-center gap-2.5 rounded-lg px-3 py-2 text-sm text-foreground transition-colors hover:bg-secondary"
              >
                <User className="h-4 w-4 text-muted-foreground" />
                Profile & Settings
              </button>

              <button
                type="button"
                role="menuitem"
                onClick={handleLogout}
                className="flex w-full items-center gap-2.5 rounded-lg px-3 py-2 text-sm text-destructive transition-colors hover:bg-destructive/10"
              >
                <LogOut className="h-4 w-4" />
                Log out
              </button>
            </div>
          )}
        </div>
      </div>
    </header>
  );
}
