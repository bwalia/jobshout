"use client";

import { useState } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { Check, Code2, Copy, Download, FileText, Newspaper } from "lucide-react";
import { cn } from "@/lib/utils/cn";
import type { BlogArticle } from "@/lib/types/blog";

type Tab = "article" | "markdown" | "html";

const TABS: { id: Tab; label: string; icon: React.ElementType }[] = [
  { id: "article", label: "Article", icon: Newspaper },
  { id: "markdown", label: "Markdown", icon: FileText },
  { id: "html", label: "HTML", icon: Code2 },
];

/**
 * Shows one generated article three ways: rendered as prose, as the raw
 * markdown the LLM wrote, and as the HTML that gets sent to the CMS.
 *
 * The markdown and HTML views are byte-for-byte what the server holds — no
 * reformatting — so either can be used to check exactly what will be published.
 * The HTML is shown as source rather than rendered: the point of the tab is to
 * see the markup, and the Article tab already covers how it reads.
 */
export function ArticleViewer({ article }: { article: BlogArticle }) {
  const [tab, setTab] = useState<Tab>("article");
  const [copied, setCopied] = useState(false);

  async function copyMarkdown() {
    try {
      await navigator.clipboard.writeText(article.markdown);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1500);
    } catch {
      // Clipboard is unavailable outside a secure context; the Download
      // button is the fallback, so failing quietly is fine here.
    }
  }

  function downloadMarkdown() {
    const blob = new Blob([article.markdown], { type: "text/markdown" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `${article.slug}.md`;
    a.click();
    URL.revokeObjectURL(url);
  }

  return (
    <div className="flex min-h-0 w-full min-w-0 flex-1 flex-col">
      {/* Header: file path + actions */}
      <div className="flex items-start justify-between gap-4 border-b border-border pb-3">
        <div className="min-w-0">
          <h2 className="truncate text-base font-semibold text-foreground">
            {article.topic}
          </h2>
          <p className="mt-0.5 truncate font-mono text-2xs text-muted-foreground">
            {article.path} · {article.word_count} words
          </p>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <button
            type="button"
            onClick={copyMarkdown}
            className="inline-flex items-center gap-1.5 rounded-md border border-border px-2.5 py-1.5 text-xs text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
          >
            {copied ? (
              <Check className="h-3.5 w-3.5" />
            ) : (
              <Copy className="h-3.5 w-3.5" />
            )}
            {copied ? "Copied" : "Copy"}
          </button>
          <button
            type="button"
            onClick={downloadMarkdown}
            className="inline-flex items-center gap-1.5 rounded-md border border-border px-2.5 py-1.5 text-xs text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
          >
            <Download className="h-3.5 w-3.5" />
            .md
          </button>
        </div>
      </div>

      {/* Tabs */}
      <div className="border-b border-border">
        <nav className="-mb-px flex gap-0" aria-label="Article view">
          {TABS.map(({ id, label, icon: Icon }) => (
            <button
              key={id}
              type="button"
              onClick={() => setTab(id)}
              className={cn(
                "inline-flex items-center gap-2 border-b-2 px-4 py-2.5 text-sm font-medium transition-colors",
                tab === id
                  ? "border-primary text-foreground"
                  : "border-transparent text-muted-foreground hover:border-border hover:text-foreground"
              )}
              aria-current={tab === id ? "page" : undefined}
            >
              <Icon className="h-4 w-4" />
              {label}
            </button>
          ))}
        </nav>
      </div>

      <div className="min-h-0 min-w-0 flex-1 overflow-y-auto scrollbar-thin pt-5">
        {tab === "article" ? (
          <article
            className={cn(
              "prose prose-sm w-full min-w-0 max-w-none dark:prose-invert",
              // Wide content scrolls inside its own box rather than widening
              // the page: long code lines and tables are common in these
              // articles and must not push the layout sideways.
              "prose-pre:overflow-x-auto",
              "[&_table]:block [&_table]:overflow-x-auto",
              // The default prose palette is its own greyscale; bind it to the
              // app's tokens so it tracks the theme like everything else.
              "prose-headings:font-display prose-headings:tracking-tight",
              "prose-a:text-primary prose-a:no-underline hover:prose-a:underline",
              "prose-code:rounded prose-code:bg-muted prose-code:px-1 prose-code:py-0.5",
              "prose-code:before:content-none prose-code:after:content-none",
              "prose-pre:border prose-pre:border-border prose-pre:bg-muted/60",
              "prose-th:text-foreground prose-blockquote:border-l-primary/40"
            )}
          >
            <ReactMarkdown remarkPlugins={[remarkGfm]}>
              {article.markdown}
            </ReactMarkdown>
          </article>
        ) : (
          <pre className="whitespace-pre-wrap break-words rounded-lg border border-border bg-muted/40 p-4 font-mono text-xs leading-relaxed text-foreground">
            {tab === "html"
              ? article.html ||
                "This article was generated before HTML conversion existed. It is converted on the way to the CMS."
              : article.markdown}
          </pre>
        )}
      </div>
    </div>
  );
}
