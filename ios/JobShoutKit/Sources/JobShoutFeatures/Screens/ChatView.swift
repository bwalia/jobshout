import JobShoutAPI
import SwiftUI

/// Sessions list + New chat. Streaming happens in `ChatSessionView`.
struct ChatView: View {
    @Environment(AppModel.self) private var app
    @State private var sessions = Resource<[ChatSession]>()
    @State private var creating = false
    @State private var createError: String?

    var body: some View {
        ResourceView(resource: sessions, retry: load) { list in
            List {
                if let createError {
                    Section {
                        Label(createError, systemImage: "exclamationmark.triangle").foregroundStyle(.red)
                    }
                }
                Section {
                    ForEach(list, id: \.id) { session in
                        NavigationLink(value: ChatRoute(id: session.id, title: "Chat")) {
                            VStack(alignment: .leading, spacing: 2) {
                                Text(title(for: session)).font(.headline)
                                Text(session.updatedAt.formatted(.relative(presentation: .named)))
                                    .font(.caption)
                                    .foregroundStyle(.secondary)
                            }
                        }
                        .accessibilityIdentifier("chat-session-row")
                    }
                    if list.isEmpty {
                        ContentUnavailableView(
                            "No chats yet",
                            systemImage: "bubble.left.and.bubble.right",
                            description: Text("Start a chat to ask JobShout to run agents and answer questions."))
                    }
                } footer: {
                    SyncFooter(loadedAt: sessions.loadedAt)
                }
            }
            .refreshable { await load() }
        }
        .navigationTitle("Chat")
        .toolbar {
            ToolbarItem(placement: .primaryAction) {
                Button {
                    Task { await newChat() }
                } label: {
                    if creating { ProgressView() } else { Image(systemName: "square.and.pencil") }
                }
                .disabled(creating)
                .accessibilityIdentifier("new-chat")
            }
        }
        .task { await load() }
    }

    private func title(for session: ChatSession) -> String {
        "Chat · \(session.createdAt.formatted(date: .abbreviated, time: .shortened))"
    }

    private func load() async {
        await sessions.load {
            try await app.api.chatSessions().sorted { $0.updatedAt > $1.updatedAt }
        }
    }

    private func newChat() async {
        creating = true
        createError = nil
        defer { creating = false }
        do {
            _ = try await app.api.startChatSession()
            await load()
        } catch {
            createError = APIError.from(error).message
        }
    }
}

struct ChatSessionView: View {
    @Environment(AppModel.self) private var app
    let sessionID: String
    var title: String = "Chat"

    @State private var messages: [ChatMessage] = []
    @State private var draft = ""
    @State private var streaming = ""
    @State private var statusLine: String?
    @State private var pendingConfirmation: ChatStreamEvent?
    @State private var clarify: ChatStreamEvent?
    @State private var loadError: String?
    @State private var sendError: String?
    @State private var sending = false

    var body: some View {
        VStack(spacing: 0) {
            ScrollViewReader { proxy in
                ScrollView {
                    LazyVStack(alignment: .leading, spacing: 12) {
                        ForEach(messages, id: \.id) { message in
                            ChatBubble(role: message.role, text: message.content)
                                .id(message.id)
                        }
                        if !streaming.isEmpty || statusLine != nil {
                            ChatBubble(role: "agent", text: streaming.isEmpty ? (statusLine ?? "…") : streaming)
                                .id("streaming")
                        }
                    }
                    .padding()
                }
                .onChange(of: messages.count) { _, _ in
                    if let last = messages.last { proxy.scrollTo(last.id, anchor: .bottom) }
                }
                .onChange(of: streaming) { _, _ in
                    proxy.scrollTo("streaming", anchor: .bottom)
                }
            }

            if let pendingConfirmation {
                confirmationBar(pendingConfirmation)
            } else if let clarify {
                clarifyBar(clarify)
            }

            if let sendError {
                Text(sendError).font(.footnote).foregroundStyle(.red).padding(.horizontal)
            }

            HStack(alignment: .bottom, spacing: 8) {
                TextField("Message", text: $draft, axis: .vertical)
                    .lineLimit(1...5)
                    .textFieldStyle(.roundedBorder)
                    .disabled(sending)
                    .accessibilityIdentifier("chat-input")
                Button {
                    Task { await send() }
                } label: {
                    Image(systemName: "arrow.up.circle.fill").font(.title)
                }
                .disabled(draft.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty || sending)
                .accessibilityIdentifier("chat-send")
            }
            .padding()
            .chromeBackground()
        }
        .navigationTitle(title)
        .inlineTitle()
        .task { await loadHistory() }
    }

    @ViewBuilder
    private func confirmationBar(_ event: ChatStreamEvent) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            Text(event.confirmationSummary ?? "Confirm this action").font(.subheadline.weight(.semibold))
            if let effect = event.confirmationEffect, !effect.isEmpty {
                Text(effect).font(.caption).foregroundStyle(.secondary)
            }
            HStack {
                Button("Approve") {
                    Task { await send(confirmationToken: event.confirmationToken) }
                }
                .buttonStyle(.borderedProminent)
                Button("Cancel") { pendingConfirmation = nil }
            }
        }
        .padding()
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(.orange.opacity(0.12))
    }

    @ViewBuilder
    private func clarifyBar(_ event: ChatStreamEvent) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            Text(event.clarifyQuestion ?? "Choose one").font(.subheadline.weight(.semibold))
            ForEach(event.clarifyOptions, id: \.value) { option in
                Button(option.label) {
                    Task { await send(content: option.value, displayContent: option.label) }
                }
                .buttonStyle(.bordered)
            }
        }
        .padding()
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(.blue.opacity(0.08))
    }

    private func loadHistory() async {
        loadError = nil
        do {
            messages = try await app.api.chatMessages(sessionID: sessionID)
        } catch {
            loadError = APIError.from(error).message
        }
    }

    private func send(
        content: String? = nil,
        confirmationToken: String? = nil,
        displayContent: String? = nil
    ) async {
        let text = (content ?? draft).trimmingCharacters(in: .whitespacesAndNewlines)
        guard !text.isEmpty || confirmationToken != nil else { return }
        sending = true
        sendError = nil
        pendingConfirmation = nil
        clarify = nil
        streaming = ""
        statusLine = "Thinking…"
        if content == nil, confirmationToken == nil {
            draft = ""
        }
        defer { sending = false }
        do {
            for try await event in app.api.streamChat(
                sessionID: sessionID,
                content: text.isEmpty ? " " : text,
                confirmationToken: confirmationToken,
                displayContent: displayContent
            ) {
                apply(event)
            }
            await loadHistory()
            streaming = ""
            statusLine = nil
        } catch {
            sendError = APIError.from(error).message
            statusLine = nil
        }
    }

    private func apply(_ event: ChatStreamEvent) {
        switch event.type {
        case "token":
            if let token = event.token { streaming += token }
            statusLine = nil
        case "tool_call":
            statusLine = event.label ?? event.tool.map { "Using \($0)…" } ?? "Working…"
        case "tool_result":
            statusLine = event.label ?? "Done with tool"
        case "model":
            if let model = event.model { statusLine = model }
        case "confirmation":
            pendingConfirmation = event
            statusLine = nil
        case "clarify":
            clarify = event
            statusLine = nil
        case "error":
            sendError = event.error ?? "Chat failed"
            statusLine = nil
        case "done":
            if streaming.isEmpty, let message = event.doneMessage {
                streaming = message
            }
            statusLine = nil
        default:
            break
        }
    }
}

private struct ChatBubble: View {
    let role: String
    let text: String

    var body: some View {
        HStack {
            if isUser { Spacer(minLength: 40) }
            Text(text)
                .padding(10)
                .background(isUser ? Color.accentColor.opacity(0.15) : Color.secondary.opacity(0.12), in: .rect(cornerRadius: 14))
                .frame(maxWidth: .infinity, alignment: isUser ? .trailing : .leading)
            if !isUser { Spacer(minLength: 40) }
        }
    }

    private var isUser: Bool { role == "user" }
}
