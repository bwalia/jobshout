"use client";

import { useEffect, useState } from "react";
import { MenuIcon, XIcon } from "@/components/icons";
import { MobileNavLinks } from "@/components/NavLinks";

export function MobileMenu({ children }: { children?: React.ReactNode }) {
  const [open, setOpen] = useState(false);

  // Escape closes; body scroll locks while the sheet is open.
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setOpen(false);
    };
    document.addEventListener("keydown", onKey);
    document.body.style.overflow = "hidden";
    return () => {
      document.removeEventListener("keydown", onKey);
      document.body.style.overflow = "";
    };
  }, [open]);

  return (
    <>
      <button
        type="button"
        onClick={() => setOpen(true)}
        aria-label="Open menu"
        aria-expanded={open}
        className="flex h-10 w-10 cursor-pointer items-center justify-center rounded-pill border border-line text-mute transition-colors duration-200 hover:border-edge hover:text-ink lg:hidden"
      >
        <MenuIcon className="h-5 w-5" />
      </button>

      {open ? (
        <div className="fixed inset-0 z-50 lg:hidden">
          <button
            type="button"
            aria-label="Close menu"
            onClick={() => setOpen(false)}
            className="absolute inset-0 cursor-default bg-ink/40 backdrop-blur-sm"
          />
          <div className="absolute inset-x-3 top-3 animate-rise-in rounded-2xl border border-line bg-surface p-3 shadow-pop">
            <div className="flex items-center justify-between px-1 pb-2">
              <p className="text-xs font-semibold uppercase tracking-[0.16em] text-mute">Menu</p>
              <button
                type="button"
                onClick={() => setOpen(false)}
                aria-label="Close menu"
                className="flex h-10 w-10 cursor-pointer items-center justify-center rounded-pill text-mute transition-colors duration-200 hover:bg-raised hover:text-ink"
              >
                <XIcon className="h-5 w-5" />
              </button>
            </div>
            <nav className="flex flex-col gap-1">
              <MobileNavLinks onNavigate={() => setOpen(false)} />
            </nav>
            <div className="mt-3 border-t border-line pt-3">{children}</div>
          </div>
        </div>
      ) : null}
    </>
  );
}
