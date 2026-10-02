import Foundation
import HTTPTypes
import OpenAPIRuntime

/// Supplies and renews the bearer token. Implemented by the session in
/// JobShoutAuth; kept as a protocol so the API layer doesn't depend on it.
public protocol AccessTokenProvider: Sendable {
    func currentAccessToken() async -> String?
    /// Called after a 401 made with `failedToken`. Returns true when a newer
    /// token is available (refreshed now, or by a concurrent caller).
    func renewAccessToken(after failedToken: String?) async -> Bool
}

/// Attaches `Authorization: Bearer` and, on a 401, renews the token once and
/// retries. Public operations go out without a token.
public struct AuthMiddleware: ClientMiddleware {
    /// Operations that must not carry a token (or retry on 401: a 401 from
    /// login is a wrong password, not an expired session).
    public static let publicOperations: Set<String> = [
        "login", "register", "refresh", "logout",
        "appleStatus", "appleNonce", "signInWithApple",
    ]

    let tokens: AccessTokenProvider

    public init(tokens: AccessTokenProvider) {
        self.tokens = tokens
    }

    public func intercept(
        _ request: HTTPRequest,
        body: HTTPBody?,
        baseURL: URL,
        operationID: String,
        next: @Sendable (HTTPRequest, HTTPBody?, URL) async throws -> (HTTPResponse, HTTPBody?)
    ) async throws -> (HTTPResponse, HTTPBody?) {
        if Self.publicOperations.contains(operationID) {
            return try await next(request, body, baseURL)
        }

        let token = await tokens.currentAccessToken()
        let (response, responseBody) = try await next(authorized(request, token), body, baseURL)
        guard response.status == .unauthorized else {
            return (response, responseBody)
        }
        // A streamed request body can't be replayed; surface the 401.
        if let body, body.iterationBehavior == .single {
            return (response, responseBody)
        }
        guard await tokens.renewAccessToken(after: token),
              let renewed = await tokens.currentAccessToken() else {
            return (response, responseBody)
        }
        return try await next(authorized(request, renewed), body, baseURL)
    }

    private func authorized(_ request: HTTPRequest, _ token: String?) -> HTTPRequest {
        var request = request
        if let token {
            request.headerFields[.authorization] = "Bearer \(token)"
        }
        return request
    }
}
