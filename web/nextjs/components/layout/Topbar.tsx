"use client";

import { useState, useRef, useEffect } from "react";
import { useRouter } from "next/navigation";
import { LogOut, ChevronDown, User } from "lucide-react";
import { useAuthStore } from "@/lib/store/auth-store";
import { clearTokens } from "@/lib/auth/auth";
import { cn } from "@/lib/utils/cn";

/**
 * Derives initials from a full name for use as an avatar placeholder.
 * e.g. "Jane Doe" → "JD", "Alice" → "A"
 */
function getInitials(fullName: string): string {
  return fullName
    .split(" ")
    .filter(Boolean)
    .slice(0, 2)
    .map((part) => part[0].toUpperCase())
    .join("");
}

export function Topbar() {
  const router = useRouter();
  const { user, logout } = useAuthStore();
  const [menuOpen, setMenuOpen] = useState(false);
  const menuRef = useRef<HTMLDivElement>(null);

  // Close the dropdown when the user clicks outside of it
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
    <header className="flex h-14 flex-shrink-0 items-center justify-between border-b border-border bg-card px-6">
      {/* Brand – visible on small screens where the sidebar may be hidden */}
      <span className="text-sm font-semibold text-foreground lg:hidden">
        Jobshout
      </span>

      {/* Right-side controls */}
      <div className="ml-auto flex items-center gap-3">
        {/* User avatar + dropdown trigger */}
        <div ref={menuRef} className="relative">
          <button
            type="button"
            onClick={() => setMenuOpen((prev) => !prev)}
            className={cn(
              "flex items-center gap-2 rounded-md px-2 py-1.5 text-sm transition-colors",
              "hover:bg-accent hover:text-accent-foreground",
              menuOpen && "bg-accent text-accent-foreground"
            )}
            aria-haspopup="true"
            aria-expanded={menuOpen}
          >
            {/* Avatar circle with initials */}
            <span
              className="flex h-7 w-7 items-center justify-center rounded-full bg-primary text-xs font-semibold text-primary-foreground"
              aria-hidden="true"
            >
              {initials}
            </span>

            {/* Display name */}
            <span className="hidden max-w-[140px] truncate sm:block">
              {user?.full_name ?? user?.email ?? "Account"}
            </span>

            <ChevronDown
              className={cn(
                "h-3.5 w-3.5 text-muted-foreground transition-transform",
                menuOpen && "rotate-180"
              )}
            />
          </button>

          {/* Dropdown menu */}
          {menuOpen && (
            <div
              role="menu"
              className={cn(
                "absolute right-0 top-full z-50 mt-1 w-52 origin-top-right",
                "rounded-md border border-border bg-popover p-1 shadow-md",
                "animate-in fade-in-0 zoom-in-95"
              )}
            >
              {/* User info block */}
              <div className="flex items-center gap-3 px-3 py-2">
                <span className="flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-full bg-primary text-xs font-semibold text-primary-foreground">
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

              {/* Profile link */}
              <button
                type="button"
                role="menuitem"
                onClick={() => {
                  setMenuOpen(false);
                  router.push("/settings");
                }}
                className="flex w-full items-center gap-2 rounded-sm px-3 py-2 text-sm text-foreground hover:bg-accent hover:text-accent-foreground"
              >
                <User className="h-4 w-4" />
                Profile &amp; Settings
              </button>

              {/* Logout */}
              <button
                type="button"
                role="menuitem"
                onClick={handleLogout}
                className="flex w-full items-center gap-2 rounded-sm px-3 py-2 text-sm text-destructive hover:bg-destructive/10"
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
