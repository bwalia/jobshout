"use client";

import { useEffect } from "react";
import { JobSupportPanel } from "./JobSupportPanel";
import { useJobSupport } from "./JobSupportProvider";

export function JobSupportDock() {
  const { dockOpen, setDockOpen, busy } = useJobSupport();

  useEffect(() => {
    if (!dockOpen) return;
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && setDockOpen(false);
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [dockOpen, setDockOpen]);

  return (
    <>
      {dockOpen && (
        <section
          className="js-dock"
          role="dialog"
          aria-label="Job Support assistant"
          aria-modal="false"
        >
          <div className="js-dock-head">
            <div>
              <div className="text-sm font-semibold text-ink">Job Support</div>
              <div className="font-mono text-[0.65rem] text-muted">
                brief · CV · private match
              </div>
            </div>
            <button
              type="button"
              className="rounded-lg px-2 py-1 text-sm text-muted hover:bg-raised hover:text-ink"
              onClick={() => setDockOpen(false)}
              aria-label="Close Job Support"
            >
              ✕
            </button>
          </div>
          <div className="js-dock-body">
            <JobSupportPanel autoFocus />
          </div>
        </section>
      )}
      <button
        type="button"
        className="js-dock-toggle"
        onClick={() => setDockOpen(!dockOpen)}
        aria-expanded={dockOpen}
        aria-label={dockOpen ? "Close Job Support" : "Open Job Support"}
      >
        <span aria-hidden="true">{dockOpen ? "✕" : "✦"}</span>
        <span className="hidden sm:inline">
          {dockOpen ? "Close" : busy ? "Matching…" : "Job Support"}
        </span>
      </button>
    </>
  );
}
