import Foundation
import HTTPTypes
@testable import JobShoutAPI
import OpenAPIRuntime
import Testing

/// Hands out tokens and counts renewals.
actor FakeTokens: AccessTokenProvider {
    var token: String?
    var renewTo: String?
    private(set) var renewCalls = 0

    init(token: String?, renewTo: String?) {
        self.token = token
        self.renewTo = renewTo
    }

    func currentAccessToken() async -> String? { token }

    func renewAccessToken(after failedToken: String?) async -> Bool {
        renewCalls += 1
        guard let renewTo else { return false }
        token = renewTo
        return true
    }
}

/// Records requests; answers 401 to any token in `rejected`.
actor Downstream {
    var seen: [String?] = []
    let rejected: Set<String>

    init(rejected: Set<String>) { self.rejected = rejected }

    func handle(_ request: HTTPRequest) -> HTTPResponse {
        let auth = request.headerFields[.authorization]
        seen.append(auth)
        if let auth, rejected.contains(auth) { return HTTPResponse(status: .unauthorized) }
        return HTTPResponse(status: .ok)
    }
}

@Suite struct AuthMiddlewareTests {
    let url = URL(string: "https://example.test/api/v1")!

    func send(_ middleware: AuthMiddleware, _ downstream: Downstream, op: String) async throws -> HTTPResponse {
        let (response, _) = try await middleware.intercept(
            HTTPRequest(method: .get, scheme: "https", authority: "example.test", path: "/x"),
            body: nil, baseURL: url, operationID: op
        ) { request, _, _ in (await downstream.handle(request), nil) }
        return response
    }

    @Test func attachesBearer() async throws {
        let down = Downstream(rejected: [])
        let r = try await send(AuthMiddleware(tokens: FakeTokens(token: "a", renewTo: nil)), down, op: "listAgents")
        #expect(r.status == .ok)
        #expect(await down.seen == ["Bearer a"])
    }

    @Test func publicOperationsGoOutWithoutTokenAndNeverRenew() async throws {
        let tokens = FakeTokens(token: "a", renewTo: "b")
        let down = Downstream(rejected: [])
        _ = try await send(AuthMiddleware(tokens: tokens), down, op: "login")
        #expect(await down.seen == [nil])
        #expect(await tokens.renewCalls == 0)
    }

    @Test func renewsOnceAndRetriesOn401() async throws {
        let tokens = FakeTokens(token: "old", renewTo: "new")
        let down = Downstream(rejected: ["Bearer old"])
        let r = try await send(AuthMiddleware(tokens: tokens), down, op: "listTasks")
        #expect(r.status == .ok)
        #expect(await down.seen == ["Bearer old", "Bearer new"])
        #expect(await tokens.renewCalls == 1)
    }

    @Test func surfaces401WhenRenewFails() async throws {
        let tokens = FakeTokens(token: "old", renewTo: nil)
        let down = Downstream(rejected: ["Bearer old"])
        let r = try await send(AuthMiddleware(tokens: tokens), down, op: "listTasks")
        #expect(r.status == .unauthorized)
        #expect(await down.seen.count == 1)
    }
}
