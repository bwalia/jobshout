import Link from "next/link";
import { UserIcon } from "@/components/icons";
import { EmptyState, buttonClass } from "@/components/ui";

export function SignInPrompt({ next, title, body }: { next: string; title: string; body: string }) {
  return (
    <EmptyState
      icon={<UserIcon className="h-6 w-6" />}
      title={title}
      body={body}
      action={
        <Link href={`/login?callbackUrl=${encodeURIComponent(next)}`} className={buttonClass("primary", "md")}>
          Sign in
        </Link>
      }
    />
  );
}
