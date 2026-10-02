import Foundation
import JobShoutAPI
import JobShoutCore

/// Keeps one authenticated WebSocket open while someone is listening and
/// reconnects with capped exponential backoff. Connection state is exposed
/// so the UI never presents stale data as live.
public final class LiveEventsClient: Sendable {
    public enum State: Sendable, Equatable { case connecting, connected, disconnected }

    public struct Update: Sendable {
        public let event: LiveEvent?
        public let state: State
    }

    private let url: URL
    private let tokens: AccessTokenProvider
    private let session: URLSession

    public init(url: URL, tokens: AccessTokenProvider, session: URLSession = .shared) {
        self.url = url
        self.tokens = tokens
        self.session = session
    }

    /// Streams events and state changes until the consuming task is cancelled.
    public func updates() -> AsyncStream<Update> {
        AsyncStream { continuation in
            let task = Task {
                var attempt = 0
                while !Task.isCancelled {
                    continuation.yield(Update(event: nil, state: .connecting))
                    let receivedAny = await runConnection(continuation)
                    continuation.yield(Update(event: nil, state: .disconnected))
                    if Task.isCancelled { break }
                    attempt = receivedAny ? 0 : attempt + 1
                    try? await Task.sleep(for: Self.backoff(attempt: attempt))
                }
                continuation.finish()
            }
            continuation.onTermination = { _ in task.cancel() }
        }
    }

    /// 1s, 2s, 4s … capped at 30s, with ±20% jitter so a fleet of phones
    /// doesn't reconnect in lockstep after a deploy.
    static func backoff(attempt: Int) -> Duration {
        let base = min(30.0, pow(2.0, Double(max(0, attempt))))
        return .milliseconds(Int(base * Double.random(in: 0.8...1.2) * 1000))
    }

    /// Returns whether the socket connected (a healthy connection resets the
    /// backoff).
    private func runConnection(_ continuation: AsyncStream<Update>.Continuation) async -> Bool {
        guard let token = await tokens.currentAccessToken() else { return false }
        var request = URLRequest(url: url)
        request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        let socket = session.webSocketTask(with: request)
        socket.resume()
        defer { socket.cancel(with: .goingAway, reason: nil) }

        var receivedAny = false
        do {
            // The server may stay quiet for minutes; a pong proves the
            // upgrade (and the token) were accepted.
            try await ping(socket)
            receivedAny = true
            continuation.yield(Update(event: nil, state: .connected))
            while !Task.isCancelled {
                let message = try await socket.receive()
                let data: Data? = switch message {
                case .string(let text): Data(text.utf8)
                case .data(let d): d
                @unknown default: nil
                }
                if let data, let event = LiveEvent.decode(data) {
                    continuation.yield(Update(event: event, state: .connected))
                }
            }
        } catch {
            if let http = socket.response as? HTTPURLResponse, http.statusCode == 401 {
                _ = await tokens.renewAccessToken(after: token)
            }
            Log.live.debug("socket closed: \(String(describing: error), privacy: .public)")
        }
        return receivedAny
    }

    private func ping(_ socket: URLSessionWebSocketTask) async throws {
        try await withCheckedThrowingContinuation { (c: CheckedContinuation<Void, Error>) in
            socket.sendPing { error in
                if let error { c.resume(throwing: error) } else { c.resume() }
            }
        }
    }
}
