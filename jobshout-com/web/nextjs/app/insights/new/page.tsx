import Link from "next/link";
import type { Metadata } from "next";
import { PenIcon } from "@/components/icons";
import { InsightForm } from "@/components/insights/InsightForm";
import { SignInPrompt } from "@/components/insights/SignInPrompt";
import { Badge, ErrorNotice } from "@/components/ui";
import { isEditor, isInsightKind, listTopics, type InsightTopic } from "@/lib/insights";
import { currentViewer } from "@/lib/session";

export const dynamic = "force-dynamic";

export const metadata: Metadata = {
  title: "Write for Insights",
  description: "Share a post, article, blog, podcast episode or video about AI and the job market.",
  robots: { index: false },
};

export default async function NewInsightPage({ searchParams }: { searchParams: { kind?: string } }) {
  const viewer = await currentViewer();
  let topics: InsightTopic[] = [];
  let error = "";
  const [editor] = await Promise.all([
    isEditor(viewer),
    listTopics().then((t) => (topics = t)).catch((e) => (error = e instanceof Error ? e.message : "Could not load topics")),
  ]);

  return (
    <div className="mx-auto max-w-3xl px-5 pb-24 pt-10 sm:px-8 sm:pt-14">
      <header>
        <Badge tone="brand">
          <PenIcon className="h-3.5 w-3.5" />
          {editor ? "Editor" : "Contributor"}
        </Badge>
        <h1 className="mt-5 font-display text-4xl font-semibold tracking-[-0.03em] text-ink sm:text-5xl">
          Write for Insights
        </h1>
        <p className="mt-4 text-base leading-relaxed text-mute">
          News, analysis and first-hand experience of AI at work.{" "}
          {editor
            ? "As an editor, what you publish goes live straight away."
            : "An editor reviews every submission before it goes live."}{" "}
          {viewer ? (
            <Link href="/insights/mine" className="font-medium text-ink underline decoration-shout/50 underline-offset-4 hover:text-shout">
              Your insights
            </Link>
          ) : null}
        </p>
      </header>

      <div className="mt-10">
        {!viewer ? (
          <SignInPrompt
            next="/insights/new"
            title="Sign in to write"
            body="We ask contributors to sign in so readers know who wrote what, and so you can edit your drafts later."
          />
        ) : error ? (
          <ErrorNotice title={error} body="The editor needs the marketplace API. Refresh in a moment." />
        ) : (
          <InsightForm
            topics={topics}
            isStaff={editor}
            defaultKind={isInsightKind(searchParams.kind) ? searchParams.kind : undefined}
          />
        )}
      </div>
    </div>
  );
}
