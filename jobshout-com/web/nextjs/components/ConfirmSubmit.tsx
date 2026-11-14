"use client";

import { useFormStatus } from "react-dom";

/** A submit button that asks first. For actions that cannot be undone. */
export function ConfirmSubmit({
  confirm: question,
  className,
  children,
}: {
  confirm: string;
  className?: string;
  children: React.ReactNode;
}) {
  const { pending } = useFormStatus();
  return (
    <button
      type="submit"
      className={className}
      disabled={pending}
      onClick={(e) => {
        if (!window.confirm(question)) e.preventDefault();
      }}
    >
      {pending ? "Working…" : children}
    </button>
  );
}
