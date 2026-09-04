"use client";

import { memo, useState } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { Check, Copy } from "lucide-react";
import { cn } from "@/lib/utils/cn";

/**
 * Agent replies are markdown — lists, tables and fenced code are routine.
 * Rendering them as plain text was the single biggest readability loss on
 * this page. Prose is bound to the app tokens so it tracks the theme, the
 * same way ArticleViewer does it.
 */
export const Markdown = memo(function Markdown({
  children,
  className,
}: {
  children: string;
  className?: string;
}) {
  return (
    <div
      className={cn(
        "prose w-full min-w-0 max-w-none dark:prose-invert",
        // 15px/1.7 rather than the app's 13px UI size. A reply is read, not
        // scanned, and the old size was the main complaint about this page.
        "text-[15px] leading-7 text-foreground",
        "prose-p:text-[15px] prose-p:leading-7 prose-li:text-[15px] prose-li:leading-7",
        "prose-strong:text-foreground prose-strong:font-semibold",
        // Chat bubbles are narrow; long code and tables scroll inside their
        // own box instead of widening the whole column.
        "prose-pre:overflow-x-auto [&_table]:block [&_table]:overflow-x-auto",
        "[&_td]:text-[14px] [&_th]:text-[14px] [&_td]:py-2 [&_th]:py-2",
        // Tighter rhythm than an article — a reply is a paragraph, not a page.
        "prose-p:my-3 prose-ul:my-3 prose-ol:my-3 prose-li:my-1",
        "prose-headings:font-display prose-headings:tracking-tight",
        "prose-h1:text-lg prose-h2:text-base prose-h3:text-[15px] prose-headings:mb-2 prose-headings:mt-4",
        "prose-a:text-primary prose-a:underline-offset-2 hover:prose-a:underline",
        "[&_:not(pre)>code]:rounded [&_:not(pre)>code]:bg-muted [&_:not(pre)>code]:px-1.5 [&_:not(pre)>code]:py-0.5 [&_:not(pre)>code]:text-[0.9em] [&_:not(pre)>code]:font-medium",
        "prose-code:before:content-none prose-code:after:content-none",
        "prose-th:text-foreground prose-blockquote:border-l-primary/40 prose-blockquote:not-italic",
        "first:[&>*]:mt-0 last:[&>*]:mb-0",
        className
      )}
    >
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        components={{
          pre({ children }) {
            return <>{children}</>;
          },
          code({ className: cls, children, node: _node, ...props }) {
            const isBlock = /language-/.test(cls ?? "");
            if (isBlock) {
              return (
                <CodeBlock
                  language={langOf(cls)}
                  code={String(children).replace(/\n$/, "")}
                />
              );
            }
            return (
              <code className={cls} {...props}>
                {children}
              </code>
            );
          },
          a({ href, children }) {
            return (
              <a href={href} target="_blank" rel="noopener noreferrer">
                {children}
              </a>
            );
          },
        }}
      >
        {normalise(children)}
      </ReactMarkdown>
    </div>
  );
});

function langOf(className?: string): string {
  const m = /language-([\w-]+)/.exec(className ?? "");
  return m?.[1] ?? "";
}

/** Fenced code with a language tag and a copy button — the two things people
 *  always want from a code answer and had to select-and-drag for before. */
function CodeBlock({ code, language }: { code: string; language: string }) {
  const [copied, setCopied] = useState(false);

  async function copy() {
    try {
      await navigator.clipboard.writeText(code);
      setCopied(true);
      setTimeout(() => setCopied(false), 1600);
    } catch {
      /* clipboard blocked — nothing useful to say */
    }
  }

  return (
    <div className="group/code relative my-3 overflow-hidden rounded-lg border border-border bg-muted/50">
      <div className="flex items-center justify-between border-b border-border/70 px-3 py-1.5">
        <span className="font-mono text-xs uppercase tracking-wide text-muted-foreground">
          {language || "code"}
        </span>
        <button
          type="button"
          onClick={() => void copy()}
          aria-label={copied ? "Copied" : "Copy code"}
          className="flex h-7 items-center gap-1.5 rounded px-2 text-xs text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          {copied ? (
            <Check className="h-3 w-3 text-signal-live" />
          ) : (
            <Copy className="h-3 w-3" />
          )}
          {copied ? "Copied" : "Copy"}
        </button>
      </div>
      {/* Typography's own `pre` box (margin, radius, fill) would sit inside
          this frame and read as a second card. */}
      <pre className="!my-0 overflow-x-auto !rounded-none !bg-transparent px-3.5 py-3 text-[13px] leading-6 !text-foreground">
        <code>{code}</code>
      </pre>
    </div>
  );
}

/**
 * The backend answers in plain text as often as markdown: bullets arrive as
 * "\u2022 " and every line is separated by a single newline. CommonMark folds
 * both into one run-on paragraph, so normalise them before parsing. Fenced
 * code is left exactly as sent.
 */
function normalise(src: string): string {
  return src
    .split(/(```[\s\S]*?```)/g)
    .map((part, i) =>
      i % 2 === 1
        ? part
        : part
            .replace(/^[ \t]*[\u2022\u25aa\u00b7][ \t]+/gm, "- ")
            .replace(/([^\n])\n(?!\n)/g, "$1  \n")
    )
    .join("");
}
