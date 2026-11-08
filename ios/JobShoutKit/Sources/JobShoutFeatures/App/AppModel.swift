import Foundation
import JobShoutAPI
import JobShoutAuth
import JobShoutCore
import JobShoutLive
import Observation

/// App-wide state: who is signed in, the API, and the live-event feed that
/// tells screens when to refetch.
@MainActor @Observable
public final class AppModel {
    public enum Phase: Equatable { case launching, signedOut, signedIn }

    public let environment: AppEnvironment
    public private(set) var phase: Phase = .launching
    public private(set) var liveState: LiveEventsClient.State = .disconnected
    /// Bumped per topic when a live event arrives; screens use it as a
    /// `.task(id:)` key to refetch.
    public private(set) var revisions: [LiveEvent.Topic: Int] = [:]
    public private(set) var pendingApprovals = 0
    /// Bumped when a push/deep-link asks to show Approvals (MainTabView selects it).
    public private(set) var approvalsFocus = 0
    public private(set) var deviceID: String?
    public private(set) var userID: String?

    public let api: PlatformAPI
    let auth: AuthService
    let session: SessionManager
    private let live: LiveEventsClient
    private var liveTask: Task<Void, Never>?
    private var pushToken: (token: String, sandbox: Bool)?

    public init(environment: AppEnvironment, store: TokenStore, device: DeviceDescriptor) {
        self.environment = environment
        let auth = AuthService(client: APIClientFactory.makeUnauthenticated(environment: environment), device: device)
        let session = SessionManager(store: store) { refreshToken in try await auth.refresh(refreshToken) }
        self.auth = auth
        self.session = session
        let tokens = session
        self.api = PlatformAPI(
            client: APIClientFactory.make(environment: environment, tokens: tokens),
            chatStream: ChatStreamClient(environment: environment, tokens: tokens)
        )
        self.live = LiveEventsClient(url: environment.liveEventsURL, tokens: tokens)
    }

    public func start() async {
        await session.onSessionEnded { [weak self] in
            Task { @MainActor in self?.didEndSession() }
        }
        if let current = await session.current {
            adopt(current)
        } else {
            phase = .signedOut
        }
    }

    public func revision(_ topic: LiveEvent.Topic) -> Int { revisions[topic, default: 0] }

    // MARK: Sign-in

    public func signIn(email: String, password: String) async throws {
        let s = try await auth.login(email: email, password: password, knownDeviceID: await session.current?.deviceID ?? deviceID)
        try await begin(s)
    }

    public func register(email: String, password: String, fullName: String, orgName: String) async throws {
        let s = try await auth.register(email: email, password: password, fullName: fullName, orgName: orgName, knownDeviceID: deviceID)
        try await begin(s)
    }

    public func appleEnabled() async -> Bool { await auth.appleEnabled() }
    public func appleNonce() async throws -> String { try await auth.appleNonce() }
    public func googleEnabled() async -> Bool { await auth.googleEnabled() }

    public func signInWithApple(identityToken: String, rawNonce: String, fullName: String?) async throws {
        let s = try await auth.signInWithApple(identityToken: identityToken, rawNonce: rawNonce, fullName: fullName, knownDeviceID: deviceID)
        try await begin(s)
    }

    public func signInWithGoogle(ticket: String) async throws {
        let s = try await auth.completeGoogle(ticket: ticket, knownDeviceID: await session.current?.deviceID ?? deviceID)
        try await begin(s)
    }

    /// Revokes this device on the server (best-effort) and forgets it locally.
    public func signOut() async {
        if let current = await session.current {
            await auth.logout(refreshToken: current.refreshToken)
        }
        await session.end()
        didEndSession()
    }

    public func signOutEverywhere() async throws {
        try await api.logoutEverywhere()
        await session.end()
        didEndSession()
    }

    // MARK: Push

    /// Stores the APNs token against this device. Called by the app delegate;
    /// held until signed in if it arrives first.
    public func didRegisterForPush(token: Data, sandbox: Bool) {
        let hex = token.map { String(format: "%02x", $0) }.joined()
        pushToken = (hex, sandbox)
        Task { await uploadPushToken() }
    }

    /// Notification tap / deep-link into Approvals. Server APNs delivery for
    /// `approval.requested` is not wired yet — this handles the payload shape
    /// once it is, and still refreshes the badge from a local/test notification.
    public func openApprovalsFromNotification(type: String? = nil) {
        approvalsFocus += 1
        revisions[.approvals, default: 0] += 1
        if let type, type.contains("approval") {
            Log.push.debug("opened approvals from push type=\(type, privacy: .public)")
        }
        Task { await refreshApprovalCount() }
    }

    private func uploadPushToken() async {
        guard phase == .signedIn, let deviceID, let pushToken else { return }
        do {
            try await api.setPushToken(deviceID: deviceID, token: pushToken.token, sandbox: pushToken.sandbox)
        } catch {
            Log.push.error("push token upload failed: \(error.localizedDescription, privacy: .public)")
        }
    }

    // MARK: Lifecycle / live

    /// Called when the app returns to the foreground. Missed WS frames while
    /// suspended cost latency only if we refetch; also restart the socket in
    /// case iOS killed it quietly in the background.
    public func noteBecameActive() {
        guard phase == .signedIn else { return }
        for topic in [LiveEvent.Topic.work, .approvals, .agents] {
            revisions[topic, default: 0] += 1
        }
        Task { await refreshApprovalCount() }
        #if !os(watchOS)
        startLive()
        #endif
    }

    // MARK: Approvals badge

    public func refreshApprovalCount() async {
        if let pending = try? await api.approvals(status: "pending") {
            pendingApprovals = pending.count
        }
    }

    // MARK: Internals

    private func begin(_ s: StoredSession) async throws {
        try await session.begin(s)
        adopt(s)
    }

    private func adopt(_ s: StoredSession) {
        userID = s.userID
        deviceID = s.deviceID
        phase = .signedIn
        #if !os(watchOS)
        // Watch uses push + short refresh while foregrounded — no WebSocket.
        startLive()
        #endif
        Task {
            await refreshApprovalCount()
            await uploadPushToken()
        }
    }

    private func didEndSession() {
        liveTask?.cancel()
        liveTask = nil
        liveState = .disconnected
        userID = nil
        pendingApprovals = 0
        phase = .signedOut
    }

    private func startLive() {
        liveTask?.cancel()
        let live = self.live
        liveTask = Task { [weak self] in
            for await update in live.updates() {
                guard let self else { return }
                // Events sent while we were disconnected are gone; refetch
                // everything once the socket is back.
                if update.state == .connected, self.liveState != .connected {
                    for topic in [LiveEvent.Topic.work, .approvals, .agents] {
                        self.revisions[topic, default: 0] += 1
                    }
                }
                self.liveState = update.state
                if let event = update.event {
                    self.revisions[event.topic, default: 0] += 1
                    if event.topic == .approvals {
                        await self.refreshApprovalCount()
                    }
                }
            }
        }
    }
}
