"use client";

import { Suspense } from "react";
import { ChatPage } from "@/components/chat/ChatPage";

export default function ChatRoutePage() {
  return (
    <Suspense
      fallback={
        <div className="flex h-screen items-center justify-center text-sm text-muted-foreground">
          Loading chat…
        </div>
      }
    >
      <ChatPage />
    </Suspense>
  );
}
