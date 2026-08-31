"use client";

import { useEffect, useRef, useState } from "react";
import { useTheme } from "next-themes";

/** Counter for unique render ids — mermaid requires one per diagram. */
let diagramSeq = 0;

/**
 * Renders one Mermaid diagram.
 *
 * Mermaid is imported dynamically rather than at module scope: it is the
 * largest dependency in the app by a wide margin, and most pages never show a
 * diagram. Loading it on demand keeps that weight off every other route.
 *
 * A diagram that fails to parse falls back to showing its own source. The
 * source is what the agent wrote and what a reader can act on — replacing it
 * with an error message would hide the only useful thing left.
 */
export function MermaidDiagram({ chart }: { chart: string }) {
  const ref = useRef<HTMLDivElement>(null);
  const [svg, setSvg] = useState<string | null>(null);
  const [failed, setFailed] = useState(false);
  const { resolvedTheme } = useTheme();

  useEffect(() => {
    let cancelled = false;

    (async () => {
      try {
        const mermaid = (await import("mermaid")).default;

        const isDark =
          resolvedTheme === "dark" ||
          document.documentElement.classList.contains("dark");

        mermaid.initialize({
          startOnLoad: false,
          theme: "base",
          securityLevel: "strict",
          fontFamily: "inherit",
          themeVariables: isDark
            ? {
                background: "transparent",
                primaryColor: "#1e293b",
                primaryTextColor: "#e2e8f0",
                primaryBorderColor: "#475569",
                lineColor: "#94a3b8",
                secondaryColor: "#334155",
                tertiaryColor: "#0f172a",
              }
            : {
                background: "transparent",
                primaryColor: "#f1f5f9",
                primaryTextColor: "#0f172a",
                primaryBorderColor: "#cbd5e1",
                lineColor: "#64748b",
                secondaryColor: "#e2e8f0",
                tertiaryColor: "#f8fafc",
              },
        });

        const id = `mermaid-${++diagramSeq}`;
        const { svg: rendered } = await mermaid.render(id, chart.trim());
        if (!cancelled) setSvg(rendered);
      } catch {
        // Invalid syntax, or mermaid failed to load. Either way the source
        // below is the useful fallback.
        if (!cancelled) setFailed(true);
      }
    })();

    return () => {
      cancelled = true;
    };
  }, [chart, resolvedTheme]);

  if (failed) {
    return (
      <figure className="my-4">
        <pre className="overflow-x-auto rounded-lg border border-border bg-muted/40 p-4 font-mono text-xs text-foreground">
          {chart}
        </pre>
        <figcaption className="mt-1 text-2xs text-muted-foreground">
          This diagram could not be rendered; its source is shown instead.
        </figcaption>
      </figure>
    );
  }

  return (
    <div
      ref={ref}
      // Wide diagrams scroll inside their own box rather than widening the
      // article, which is the same rule the code blocks and tables follow.
      className="my-4 overflow-x-auto rounded-lg border border-border bg-background/40 p-4 [&_svg]:mx-auto [&_svg]:h-auto [&_svg]:max-w-full"
      // Mermaid output is generated from the diagram source with
      // securityLevel "strict", which strips scripts and inline handlers.
      dangerouslySetInnerHTML={svg ? { __html: svg } : undefined}
    >
      {svg ? undefined : (
        <p className="text-center text-xs text-muted-foreground">
          Rendering diagram…
        </p>
      )}
    </div>
  );
}
