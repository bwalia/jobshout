"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { useTheme } from "next-themes";
import { ArrowRight, Moon, Play, Sun } from "lucide-react";
import { useUiStore } from "@/lib/store/ui-store";
import {
  TEXT_SIZES,
  applyTextSize,
  readTextSize,
  type TextSize,
} from "@/lib/text-size";
import { cn } from "@/lib/utils/cn";

const HOW_TO: { title: string; steps: string[]; href?: string; cta: string }[] = [
  {
    title: "Ask for something in your own words",
    steps: [
      "Open Chat.",
      "Type what you need, as you would to a colleague — for example “Write a blog post about winter heating costs”.",
      "The assistant asks for anything it is missing, then hands the work to the right agent.",
    ],
    href: "/chat",
    cta: "Open Chat",
  },
  {
    title: "Give an agent a job directly",
    steps: [
      "Press “Run a task” (below, or at the top of most pages).",
      "Choose the agent, fill in the short form and press Run.",
      "The job appears in Tasks straight away, so you can watch it.",
    ],
    cta: "Run a task",
  },
  {
    title: "See what your agents are doing",
    steps: [
      "Open Tasks. Every job is listed with where it has got to.",
      "Click a task to see its progress, its result and anything that went wrong.",
      "A red number in the menu means something failed and needs a look.",
    ],
    href: "/panel/tasks",
    cta: "Open Tasks",
  },
  {
    title: "Find finished work",
    steps: [
      "Open Outputs for articles, pictures and videos your agents made.",
      "Click any item to open it, copy it or download it.",
    ],
    href: "/panel/artifacts",
    cta: "Open Outputs",
  },
  {
    title: "When something goes wrong",
    steps: [
      "Open the task. The reason it stopped is shown in a red box.",
      "Most problems are fixed by pressing “Retry” in that box.",
      "If it fails again, the reason will usually name what to change — a missing setting, or a service that was busy.",
    ],
    href: "/panel/runs",
    cta: "See all runs",
  },
  {
    title: "Check what it is costing",
    steps: ["Open Usage to see spend by day, by agent and by model."],
    href: "/panel/usage",
    cta: "Open Usage",
  },
];

const WORDS: [string, string][] = [
  ["Agent", "A helper that does one kind of job, such as writing articles or making videos."],
  ["Task", "One job you have given an agent. It moves from To do, to In progress, to Done."],
  ["Run", "One attempt at a task. Running a task again starts a new run."],
  ["Project", "A folder that groups related tasks together."],
  ["Output", "Something an agent made: an article, a picture, a video or a file."],
  ["Model", "The AI engine an agent thinks with. You rarely need to change it."],
  ["Schedule", "A task set to run by itself at a set time, such as every Monday at 9am."],
];

export default function HelpPage() {
  const openRunDialog = useUiStore((s) => s.openRunDialog);
  const { resolvedTheme, setTheme } = useTheme();
  const [size, setSize] = useState<TextSize>("standard");
  const [mounted, setMounted] = useState(false);
  useEffect(() => {
    setSize(readTextSize());
    setMounted(true);
  }, []);

  return (
    <div className="mx-auto max-w-3xl space-y-10 p-6">
      <header>
        <h1>Help</h1>
        <p className="mt-2 text-base text-muted-foreground">
          JobShout gives your work to AI helpers, called agents, and shows you
          how each job is going. Here is how to do the common things.
        </p>
      </header>

      <section aria-labelledby="comfort" className="rounded-xl border border-border bg-card p-5">
        <h2 id="comfort" className="text-xl">Make it easier to read</h2>
        <p className="mt-1 text-base text-muted-foreground">
          These choices are remembered on this computer.
        </p>
        <fieldset className="mt-4">
          <legend className="text-base font-semibold">Text size</legend>
          <div className="mt-2 flex flex-wrap gap-2">
            {TEXT_SIZES.map((t, i) => (
              <button
                key={t.value}
                type="button"
                aria-pressed={size === t.value}
                onClick={() => {
                  applyTextSize(t.value);
                  setSize(t.value);
                }}
                className={cn(
                  "min-h-12 rounded-lg border-2 px-5 font-semibold",
                  size === t.value
                    ? "border-primary bg-primary text-primary-foreground"
                    : "border-border bg-background hover:bg-secondary"
                )}
                style={{ fontSize: `${1 + i * 0.125}rem` }}
              >
                {t.label}
              </button>
            ))}
          </div>
        </fieldset>
        <fieldset className="mt-5">
          <legend className="text-base font-semibold">Colours</legend>
          <div className="mt-2 flex flex-wrap gap-2">
            {(["light", "dark"] as const).map((t) => (
              <button
                key={t}
                type="button"
                aria-pressed={mounted && resolvedTheme === t}
                onClick={() => setTheme(t)}
                className={cn(
                  "inline-flex min-h-12 items-center gap-2 rounded-lg border-2 px-5 text-base font-semibold",
                  mounted && resolvedTheme === t
                    ? "border-primary bg-primary text-primary-foreground"
                    : "border-border bg-background hover:bg-secondary"
                )}
              >
                {t === "light" ? <Sun className="h-5 w-5" /> : <Moon className="h-5 w-5" />}
                {t === "light" ? "Light" : "Dark"}
              </button>
            ))}
          </div>
        </fieldset>
        <p className="mt-4 text-sm text-muted-foreground">
          The same buttons sit at the bottom of the menu on every page.
        </p>
      </section>

      <section aria-labelledby="howto" className="space-y-4">
        <h2 id="howto" className="text-xl">How do I…</h2>
        {HOW_TO.map((h) => (
          <div key={h.title} className="rounded-xl border border-border bg-card p-5">
            <h3>{h.title}</h3>
            <ol className="mt-2 list-decimal space-y-1.5 pl-6 text-base">
              {h.steps.map((s) => (
                <li key={s}>{s}</li>
              ))}
            </ol>
            {h.href ? (
              <Link
                href={h.href}
                className="mt-4 inline-flex min-h-11 items-center gap-2 rounded-lg border border-border bg-background px-4 text-base font-semibold hover:bg-secondary"
              >
                {h.cta} <ArrowRight className="h-4 w-4" />
              </Link>
            ) : (
              <button
                type="button"
                onClick={() => openRunDialog()}
                className="mt-4 inline-flex min-h-11 items-center gap-2 rounded-lg bg-primary px-4 text-base font-semibold text-primary-foreground hover:bg-primary/90"
              >
                <Play className="h-4 w-4" /> {h.cta}
              </button>
            )}
          </div>
        ))}
      </section>

      <section aria-labelledby="words">
        <h2 id="words" className="text-xl">Words used here</h2>
        <dl className="mt-3 divide-y divide-border rounded-xl border border-border bg-card">
          {WORDS.map(([word, meaning]) => (
            <div key={word} className="grid gap-1 p-4 sm:grid-cols-[9rem_1fr]">
              <dt className="text-base font-semibold">{word}</dt>
              <dd className="text-base text-muted-foreground">{meaning}</dd>
            </div>
          ))}
        </dl>
      </section>

      <section aria-labelledby="keys">
        <h2 id="keys" className="text-xl">Shortcuts</h2>
        <ul className="mt-3 space-y-2 text-base">
          <li>
            <kbd className="rounded border border-border px-1.5 font-mono">Ctrl</kbd> or{" "}
            <kbd className="rounded border border-border px-1.5 font-mono">⌘</kbd> +{" "}
            <kbd className="rounded border border-border px-1.5 font-mono">K</kbd> — search for
            any page, agent or task.
          </li>
          <li>
            <kbd className="rounded border border-border px-1.5 font-mono">Ctrl</kbd> or{" "}
            <kbd className="rounded border border-border px-1.5 font-mono">⌘</kbd> +{" "}
            <kbd className="rounded border border-border px-1.5 font-mono">N</kbd> — start a new
            chat.
          </li>
        </ul>
      </section>
    </div>
  );
}
