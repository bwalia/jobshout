import Foundation
import JobShoutAPI
import JobShoutCore

/// Owns the tokens. Every request asks it for the access token; a 401 asks it
/// to renew, and concurrent 401s share one refresh (refresh tokens rotate, so
/// two parallel refreshes would revoke each other).
public actor SessionManager: AccessTokenProvider {
    public typealias Refresher = @Sendable (_ refreshToken: String) async throws -> RefreshOutcome

    public enum RefreshOutcome: Sendable {
        case renewed(StoredSession)
        /// The server refused the refresh token: the session is over.
        case rejected
    }

    private let store: TokenStore
    private let refresher: Refresher
    private var session: StoredSession?
    private var inFlight: Task<Bool, Never>?
    private var endedHandlers: [@Sendable () -> Void] = []

    public init(store: TokenStore, refresher: @escaping Refresher) {
        self.store = store
        self.refresher = refresher
        self.session = try? store.load()
    }

    public var current: StoredSession? { session }

    /// Called when the session ends without the user asking (refresh
    /// rejected). The app uses it to return to sign-in.
    public func onSessionEnded(_ handler: @escaping @Sendable () -> Void) {
        endedHandlers.append(handler)
    }

    public func begin(_ newSession: StoredSession) throws {
        try store.save(newSession)
        session = newSession
    }

    /// Clears local state. Revoking on the server is the caller's job and is
    /// best-effort; the local session goes regardless.
    public func end() {
        inFlight?.cancel()
        inFlight = nil
        session = nil
        do { try store.clear() } catch { Log.auth.error("keychain clear failed: \(String(describing: error), privacy: .public)") }
    }

    // MARK: AccessTokenProvider

    public func currentAccessToken() async -> String? {
        session?.accessToken
    }

    public func renewAccessToken(after failedToken: String?) async -> Bool {
        guard let current = session else { return false }
        // Someone already renewed since this request was sent.
        if let failedToken, current.accessToken != failedToken { return true }

        if let inFlight { return await inFlight.value }
        let refreshToken = current.refreshToken
        let task = Task { await self.performRefresh(refreshToken) }
        inFlight = task
        let ok = await task.value
        inFlight = nil
        return ok
    }

    private func performRefresh(_ refreshToken: String) async -> Bool {
        do {
            switch try await refresher(refreshToken) {
            case .renewed(let renewed):
                // Signed out while the refresh was in flight: don't resurrect.
                guard session?.refreshToken == refreshToken else { return false }
                try begin(renewed)
                return true
            case .rejected:
                Log.auth.notice("refresh rejected; ending session")
                end()
                for handler in endedHandlers { handler() }
                return false
            }
        } catch {
            // Offline or a server error: keep the session and let the caller
            // surface the failure. Retrying later may still succeed.
            Log.auth.error("refresh failed: \(String(describing: error), privacy: .public)")
            return false
        }
    }
}
