"use client";

import { useState } from "react";
import { Download } from "lucide-react";
import { toast } from "sonner";
import { exportAgent } from "@/lib/api/agents";

export function ExportAgentButton({
  agentId,
  className,
}: {
  agentId: string;
  className?: string;
}) {
  const [busy, setBusy] = useState(false);

  async function onExport() {
    setBusy(true);
    try {
      const { blob, filename, warnings } = await exportAgent(agentId);
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = filename;
      document.body.appendChild(a);
      a.click();
      a.remove();
      URL.revokeObjectURL(url);
      if (warnings.length > 0) {
        toast.message("Agent exported", {
          description: warnings[0],
        });
      } else {
        toast.success("Agent exported.");
      }
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to export agent");
    } finally {
      setBusy(false);
    }
  }

  return (
    <button
      type="button"
      onClick={() => void onExport()}
      disabled={busy}
      className={
        className ??
        "inline-flex h-9 items-center gap-1.5 rounded-md border border-border px-3 text-sm hover:bg-secondary disabled:opacity-50"
      }
    >
      <Download className="h-4 w-4" />
      {busy ? "Exporting…" : "Export"}
    </button>
  );
}
