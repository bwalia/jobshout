import Link from "next/link";
import type { Metadata } from "next";
import { PlusIcon } from "@/components/icons";
import { SignInPrompt } from "@/components/insights/SignInPrompt";
import { AppForm } from "@/components/showcase/AppForm";
import { Badge } from "@/components/ui";
import { isEditor } from "@/lib/insights";
import { currentViewer } from "@/lib/session";

export const dynamic = "force-dynamic";

export const metadata: Metadata = {
  title: "Showcase your app",
  description: "Show what you built with AI: the app, the agents behind it, and the evidence it is ready.",
  robots: { index: false },
};

export default async function NewAppPage() {
  const viewer = await currentViewer();
  const editor = await isEditor(viewer);

  return (
    <div className="mx-auto max-w-3xl px-5 pb-24 pt-10 sm:px-8 sm:pt-14">
      <header>
        <Badge tone="brand">
          <PlusIcon className="h-3.5 w-3.5" />
          AI Showcase
        </Badge>
        <h1 className="mt-5 font-display text-4xl font-semibold tracking-[-0.03em] text-ink sm:text-5xl">
          Showcase your app
        </h1>
        <p className="mt-4 text-base leading-relaxed text-mute">
          Show what you built, who (and which agents) built it, and how ready it is.{" "}
          {editor
            ? "As an editor, what you publish goes live straight away."
            : "An editor checks every app and its links before it goes live."}{" "}
          {viewer ? (
            <Link href="/showcase/mine" className="font-medium text-ink underline decoration-shout/50 underline-offset-4 hover:text-shout">
              Your apps
            </Link>
          ) : null}
        </p>
      </header>

      <div className="mt-10">
        {!viewer ? (
          <SignInPrompt
            next="/showcase/new"
            title="Sign in to add your app"
            body="Apps are tied to your account so you can edit them, and so visitors know who built what."
          />
        ) : (
          <AppForm isStaff={editor} />
        )}
      </div>
    </div>
  );
}
