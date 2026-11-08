import Foundation
import JobShoutCore

/// One SSE event from `POST /chat/sessions/{id}/messages/stream`.
public struct ChatStreamEvent: Sendable, Equatable {
    public var type: String
    public var token: String?
    public var tool: String?
    public var label: String?
    public var status: String?
    public var error: String?
    public var model: String?
    public var confirmationToken: String?
    public var confirmationSummary: String?
    public var confirmationEffect: String?
    public var clarifyQuestion: String?
    public var clarifyOptions: [(label: String, value: String)]
    public var doneMessage: String?

    public init(
        type: String,
        token: String? = nil,
        tool: String? = nil,
        label: String? = nil,
        status: String? = nil,
        error: String? = nil,
        model: String? = nil,
        confirmationToken: String? = nil,
        confirmationSummary: String? = nil,
        confirmationEffect: String? = nil,
        clarifyQuestion: String? = nil,
        clarifyOptions: [(label: String, value: String)] = [],
        doneMessage: String? = nil
    ) {
        self.type = type
        self.token = token
        self.tool = tool
        self.label = label
        self.status = status
        self.error = error
        self.model = model
        self.confirmationToken = confirmationToken
        self.confirmationSummary = confirmationSummary
        self.confirmationEffect = confirmationEffect
        self.clarifyQuestion = clarifyQuestion
        self.clarifyOptions = clarifyOptions
        self.doneMessage = doneMessage
    }

    public static func == (lhs: ChatStreamEvent, rhs: ChatStreamEvent) -> Bool {
        lhs.type == rhs.type
            && lhs.token == rhs.token
            && lhs.tool == rhs.tool
            && lhs.label == rhs.label
            && lhs.status == rhs.status
            && lhs.error == rhs.error
            && lhs.model == rhs.model
            && lhs.confirmationToken == rhs.confirmationToken
            && lhs.confirmationSummary == rhs.confirmationSummary
            && lhs.confirmationEffect == rhs.confirmationEffect
            && lhs.clarifyQuestion == rhs.clarifyQuestion
            && lhs.doneMessage == rhs.doneMessage
            && lhs.clarifyOptions.map(\.label) == rhs.clarifyOptions.map(\.label)
            && lhs.clarifyOptions.map(\.value) == rhs.clarifyOptions.map(\.value)
    }
}

/// SSE-over-POST for chat. The generated OpenAPI client does not consume
/// `text/event-stream`; this uses URLSession with the same bearer renew path.
public struct ChatStreamClient: Sendable {
    private let environment: AppEnvironment
    private let tokens: AccessTokenProvider
    private let session: URLSession

    public init(environment: AppEnvironment, tokens: AccessTokenProvider, session: URLSession = .shared) {
        self.environment = environment
        self.tokens = tokens
        self.session = session
    }

    public func stream(
        sessionID: String,
        content: String,
        source: String = "ios",
        confirmationToken: String? = nil,
        displayContent: String? = nil
    ) -> AsyncThrowingStream<ChatStreamEvent, Error> {
        AsyncThrowingStream { continuation in
            let task = Task {
                do {
                    try await run(
                        sessionID: sessionID,
                        content: content,
                        source: source,
                        confirmationToken: confirmationToken,
                        displayContent: displayContent,
                        continuation: continuation,
                        retried: false
                    )
                    continuation.finish()
                } catch {
                    continuation.finish(throwing: error)
                }
            }
            continuation.onTermination = { _ in task.cancel() }
        }
    }

    private func run(
        sessionID: String,
        content: String,
        source: String,
        confirmationToken: String?,
        displayContent: String?,
        continuation: AsyncThrowingStream<ChatStreamEvent, Error>.Continuation,
        retried: Bool
    ) async throws {
        guard let token = await tokens.currentAccessToken() else {
            throw APIError.undocumented(401)
        }
        var request = URLRequest(url: streamURL(sessionID: sessionID))
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        request.setValue("text/event-stream", forHTTPHeaderField: "Accept")
        var body: [String: String] = ["content": content, "source": source]
        if let confirmationToken, !confirmationToken.isEmpty {
            body["confirmation_token"] = confirmationToken
        }
        if let displayContent, !displayContent.isEmpty {
            body["display_content"] = displayContent
        }
        request.httpBody = try JSONSerialization.data(withJSONObject: body)

        let (bytes, response) = try await session.bytes(for: request)
        guard let http = response as? HTTPURLResponse else {
            throw APIError.undocumented(0)
        }
        if http.statusCode == 401, !retried,
           await tokens.renewAccessToken(after: token) {
            try await run(
                sessionID: sessionID,
                content: content,
                source: source,
                confirmationToken: confirmationToken,
                displayContent: displayContent,
                continuation: continuation,
                retried: true
            )
            return
        }
        guard (200..<300).contains(http.statusCode) else {
            throw APIError.undocumented(http.statusCode)
        }

        var eventType = ""
        var dataLines: [String] = []
        for try await line in bytes.lines {
            if Task.isCancelled { return }
            if line.hasPrefix("event:") {
                eventType = String(line.dropFirst(6)).trimmingCharacters(in: .whitespaces)
            } else if line.hasPrefix("data:") {
                dataLines.append(String(line.dropFirst(5)).trimmingCharacters(in: .whitespaces))
            } else if line.isEmpty {
                if let event = Self.parse(type: eventType, data: dataLines.joined(separator: "\n")) {
                    continuation.yield(event)
                }
                eventType = ""
                dataLines = []
            }
        }
        if let event = Self.parse(type: eventType, data: dataLines.joined(separator: "\n")) {
            continuation.yield(event)
        }
    }

    private func streamURL(sessionID: String) -> URL {
        environment.apiBaseURL
            .appending(path: "chat/sessions/\(sessionID)/messages/stream")
    }

    static func parse(type: String, data: String) -> ChatStreamEvent? {
        let trimmed = data.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty, let raw = trimmed.data(using: .utf8) else { return nil }
        guard let obj = try? JSONSerialization.jsonObject(with: raw) as? [String: Any] else { return nil }
        let resolvedType = (obj["type"] as? String)?.nilIfBlank ?? type.nilIfBlank ?? "token"
        var event = ChatStreamEvent(type: resolvedType)
        event.token = obj["token"] as? String
        event.tool = obj["tool"] as? String
        event.label = obj["label"] as? String
        event.status = obj["status"] as? String
        event.error = obj["error"] as? String
        event.model = obj["model"] as? String
        if let confirmation = obj["confirmation"] as? [String: Any] {
            event.confirmationToken = confirmation["token"] as? String
            event.confirmationSummary = confirmation["summary"] as? String
            event.confirmationEffect = confirmation["effect"] as? String
        }
        if let clarify = obj["clarify"] as? [String: Any] {
            event.clarifyQuestion = clarify["question"] as? String
            if let options = clarify["options"] as? [[String: Any]] {
                event.clarifyOptions = options.compactMap { opt in
                    guard let label = opt["label"] as? String, let value = opt["value"] as? String else { return nil }
                    return (label, value)
                }
            }
        }
        if let response = obj["response"] as? [String: Any] {
            event.doneMessage = response["message"] as? String
        }
        return event
    }
}

private extension String {
    var nilIfBlank: String? {
        let t = trimmingCharacters(in: .whitespacesAndNewlines)
        return t.isEmpty ? nil : t
    }
}
