"use client";

import { ChatPage } from "@/components/chat/ChatPage";

export default function ChatRoutePage() {
  return (
    <div>
      <h1 className="mb-4 font-display text-2xl font-semibold tracking-tight">
        Chat
      </h1>
      <ChatPage />
    </div>
  );
}
