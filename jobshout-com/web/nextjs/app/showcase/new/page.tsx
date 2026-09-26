import Link from "next/link";
import type { Metadata } from "next";
import { BotIcon, LayersIcon, PlusIcon, RocketIcon } from "@/components/icons";
import { SignInPrompt } from "@/components/insights/SignInPrompt";
import { AppForm } from "@/components/showcase/AppForm";
import { Badge, cx } from "@/components/ui";
import { isEditor } from "@/lib/insights";
import { isKind, linkableJobs, type Kind } from "@/lib/showcase";
import { currentViewer } from "@/lib/session";

export const dynamic = "force-dynamic";

export const metadata: Metadata = {
  title: "Add to the AI Showcase",
  description: "Show what you built with AI: the app, the agents behind it, and the evidence it is ready.",
  robots: { index: false },
};

const CHOICES: Array<{ kind: Kind; title: string; blurb: string; icon: React.ReactNode }> = [
  { kind: "app", title: "An app", blurb: "Software you built, with who and which agents built it.", icon: <RocketIcon className="h-5 w-5" /> },
  { kind: "agent", title: "An agent", blurb: "What it runs on, what it can do, and the apps it built.", icon: <BotIcon className="h-5 w-5" /> },
  { kind: "team", title: "An agent team", blurb: "Agents from the directory, in the order the work flows.", icon: <LayersIcon className="h-5 w-5" /> },
];

const HEADINGS: Record<Kind, { title: string; lead: string }> = {
  app: { title: "Showcase your app", lead: "Show what you built, who (and which agents) built it, and how ready it is." },
  agent: { title: "Add an agent", lead: "List an agent in the directory, then link it from the apps it built." },
  team: { title: "Add an agent team", lead: "Group directory agents into a team and show how the work moves between them." },
};

export default async function NewEntryPage({ searchParams }: { searchParams: { kind?: string } }) {
  const viewer = await currentViewer();
  const [editor, jobs] = await Promise.all([isEditor(viewer), viewer ? linkableJobs(viewer) : Promise.resolve([])]);
  const kind: Kind = isKind(searchParams.kind) ? searchParams.kind : "app";

  return (
    <div className="mx-auto max-w-3xl px-5 pb-24 pt-10 sm:px-8 sm:pt-14">
      <header>
        <Badge tone="brand">
          <PlusIcon className="h-3.5 w-3.5" />
          AI Showcase
        </Badge>
        <h1 className="mt-5 font-display text-4xl font-semibold tracking-[-0.03em] text-ink sm:text-5xl">
          {HEADINGS[kind].title}
        </h1>
        <p className="mt-4 text-base leading-relaxed text-mute">
          {HEADINGS[kind].lead}{" "}
          {editor
            ? "As an editor, what you publish goes live straight away."
            : "An editor checks everything and its links before it goes live."}{" "}
          {viewer ? (
            <Link href="/showcase/mine" className="font-medium text-ink underline decoration-shout/50 underline-offset-4 hover:text-shout">
              Your showcase
            </Link>
          ) : null}
        </p>
      </header>

      <nav aria-label="What are you adding?" className="mt-8">
        <ul className="grid gap-2.5 sm:grid-cols-3">
          {CHOICES.map((c) => {
            const active = c.kind === kind;
            return (
              <li key={c.kind}>
                <Link
                  href={`/showcase/new?kind=${c.kind}`}
                  aria-current={active ? "page" : undefined}
                  className={cx(
                    "flex h-full flex-col gap-1.5 rounded-xl border p-3.5 transition-colors duration-200",
                    active ? "border-shout bg-shout/[0.07]" : "border-line bg-surface hover:border-edge",
                  )}
                >
                  <span className={active ? "text-shout" : "text-mute"}>{c.icon}</span>
                  <span className="text-sm font-semibold text-ink">{c.title}</span>
                  <span className="text-xs leading-relaxed text-mute">{c.blurb}</span>
                </Link>
              </li>
            );
          })}
        </ul>
      </nav>

      <div className="mt-10">
        {!viewer ? (
          <SignInPrompt
            next={`/showcase/new?kind=${kind}`}
            title="Sign in to add to the showcase"
            body="Listings are tied to your account so you can edit them, and so visitors know who built what."
          />
        ) : (
          // Keyed so switching kind starts a fresh form.
          <AppForm key={kind} isStaff={editor} kind={kind} jobs={jobs} />
        )}
      </div>
    </div>
  );
}
