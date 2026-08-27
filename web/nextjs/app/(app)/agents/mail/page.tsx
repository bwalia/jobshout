import { Metadata } from "next";
import { Suspense } from "react";
import { Mail } from "lucide-react";
import { MailAgentClient } from "@/components/MailAgentClient";

export const metadata: Metadata = {
  title: "Mail Agent",
  description: "Watch the organisation Gmail inbox, draft replies, approve before send",
};

export default function MailAgentPage() {
  return (
    <div className="space-y-6">
      <div>
        <div className="mb-1 flex items-center gap-3">
          <Mail className="h-7 w-7 text-primary" />
          <h1 className="text-2xl font-semibold tracking-tight">Mail Agent</h1>
        </div>
        <p className="text-sm text-muted-foreground">
          Watches the shared organisation mailbox, drafts replies, and asks Research
          when a reply needs facts. Nothing is sent until you approve.
        </p>
      </div>

      <div className="grid gap-6 lg:grid-cols-3">
        <div className="lg:col-span-2">
          <Suspense fallback={<p className="text-sm text-muted-foreground">Loading Mail Agent…</p>}>
            <MailAgentClient />
          </Suspense>
        </div>
        <div className="space-y-4">
          <div className="rounded-xl border border-border bg-card p-4 text-card-foreground">
            <h3 className="mb-3 text-sm font-semibold">How it works</h3>
            <ol className="list-decimal space-y-2 pl-4 text-sm text-muted-foreground">
              <li>Connect one shared Gmail account</li>
              <li>Sync pulls unread mail from the last week</li>
              <li>Mail Agent classifies and drafts a reply</li>
              <li>You edit, then Approve (sends) or Reject</li>
            </ol>
          </div>
          <div className="rounded-xl border border-border bg-accent p-4">
            <p className="text-xs text-accent-foreground">
              <strong>Draft only.</strong> The agent never sends until a person
              clicks Approve on that draft.
            </p>
          </div>
        </div>
      </div>
    </div>
  );
}
