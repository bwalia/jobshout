import Foundation
@testable import JobShoutAuth
import Testing

actor RefreshCounter {
    var calls = 0
    func hit() { calls += 1 }
}

@Suite struct SessionManagerTests {
    static let initial = StoredSession(accessToken: "a1", refreshToken: "r1", deviceID: "d", userID: "u")

    @Test func concurrent401sShareOneRefresh() async {
        let counter = RefreshCounter()
        let manager = SessionManager(store: InMemoryTokenStore(Self.initial)) { refresh in
            await counter.hit()
            try await Task.sleep(for: .milliseconds(50))
            return .renewed(StoredSession(accessToken: "a2", refreshToken: "r2", deviceID: "d", userID: "u"))
        }
        let results = await withTaskGroup(of: Bool.self) { group in
            for _ in 0..<8 { group.addTask { await manager.renewAccessToken(after: "a1") } }
            return await group.reduce(into: [Bool]()) { $0.append($1) }
        }
        #expect(results.allSatisfy { $0 })
        #expect(await counter.calls == 1)
        #expect(await manager.currentAccessToken() == "a2")
    }

    @Test func staleFailureDoesNotRefreshAgain() async {
        let counter = RefreshCounter()
        let manager = SessionManager(store: InMemoryTokenStore(Self.initial)) { _ in
            await counter.hit()
            return .rejected
        }
        // A request that failed with a token we've since replaced.
        #expect(await manager.renewAccessToken(after: "older-token"))
        #expect(await counter.calls == 0)
    }

    @Test func rejectedRefreshEndsTheSession() async throws {
        let store = InMemoryTokenStore(Self.initial)
        let manager = SessionManager(store: store) { _ in .rejected }
        let ended = RefreshCounter()
        await manager.onSessionEnded { Task { await ended.hit() } }

        #expect(await manager.renewAccessToken(after: "a1") == false)
        #expect(await manager.current == nil)
        #expect(try store.load() == nil)
        try await Task.sleep(for: .milliseconds(20))
        #expect(await ended.calls == 1)
    }

    @Test func networkFailureKeepsTheSession() async {
        struct Offline: Error {}
        let manager = SessionManager(store: InMemoryTokenStore(Self.initial)) { _ in throw Offline() }
        #expect(await manager.renewAccessToken(after: "a1") == false)
        #expect(await manager.current == Self.initial)
    }

    @Test func appleNonceHashIsLowercaseSHA256Hex() {
        // Known vector; the server compares against the same encoding.
        #expect(AppleNonce.hash("abc") == "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad")
    }
}
