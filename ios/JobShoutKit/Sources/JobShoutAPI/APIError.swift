import Foundation
import OpenAPIRuntime

/// What a screen shows when a call fails.
public struct APIError: Error, LocalizedError, Equatable, Sendable {
    public let status: Int?
    public let message: String

    public init(status: Int?, message: String) {
        self.status = status
        self.message = message
    }

    public var errorDescription: String? { message }

    public var isUnauthorized: Bool { status == 401 }

    /// A documented error response with the server's `{error}` body.
    public static func server(_ status: Int, _ body: Components.Schemas._Error?) -> APIError {
        APIError(status: status, message: body?.error ?? HTTPURLResponse.localizedString(forStatusCode: status).capitalized)
    }

    public static func undocumented(_ status: Int) -> APIError {
        switch status {
        case 401: APIError(status: 401, message: "Your session has ended. Sign in again.")
        case 403: APIError(status: 403, message: "You don't have permission to do that.")
        case 404: APIError(status: 404, message: "That no longer exists.")
        case 500...: APIError(status: status, message: "JobShout had a problem (\(status)). Try again shortly.")
        default: APIError(status: status, message: "Request failed (\(status)).")
        }
    }

    /// Maps anything thrown by a client call to something presentable.
    public static func from(_ error: Error) -> APIError {
        if let api = error as? APIError { return api }
        if let client = error as? ClientError {
            if let url = client.underlyingError as? URLError { return .network(url) }
            if client.underlyingError is DecodingError {
                return APIError(status: nil, message: "JobShout sent a response this version of the app can't read.")
            }
        }
        if let url = error as? URLError { return .network(url) }
        return APIError(status: nil, message: error.localizedDescription)
    }

    static func network(_ error: URLError) -> APIError {
        switch error.code {
        case .notConnectedToInternet, .networkConnectionLost, .dataNotAllowed:
            APIError(status: nil, message: "You're offline.")
        case .timedOut:
            APIError(status: nil, message: "JobShout took too long to answer.")
        case .serverCertificateUntrusted, .serverCertificateHasBadDate, .serverCertificateNotYetValid, .secureConnectionFailed:
            APIError(status: nil, message: "Couldn't establish a secure connection to JobShout.")
        default:
            APIError(status: nil, message: error.localizedDescription)
        }
    }
}
