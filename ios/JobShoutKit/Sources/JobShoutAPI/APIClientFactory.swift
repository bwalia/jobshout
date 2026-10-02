import Foundation
import JobShoutCore
import OpenAPIRuntime
import OpenAPIURLSession

public enum APIClientFactory {
    /// The client every feature uses: bearer auth with renew-and-retry.
    public static func make(
        environment: AppEnvironment,
        tokens: AccessTokenProvider,
        session: URLSession = .shared
    ) -> Client {
        Client(
            serverURL: environment.apiBaseURL,
            configuration: configuration,
            transport: URLSessionTransport(configuration: .init(session: session)),
            middlewares: [AuthMiddleware(tokens: tokens)]
        )
    }

    /// A client with no auth middleware, for the session itself (refresh,
    /// sign-in), so renewing a token can never recurse into renewing it.
    public static func makeUnauthenticated(
        environment: AppEnvironment,
        session: URLSession = .shared
    ) -> Client {
        Client(
            serverURL: environment.apiBaseURL,
            configuration: configuration,
            transport: URLSessionTransport(configuration: .init(session: session))
        )
    }

    /// Go encodes times as RFC 3339 with fractional seconds; accept both.
    static let configuration = Configuration(dateTranscoder: FlexibleISO8601DateTranscoder())
}

struct FlexibleISO8601DateTranscoder: DateTranscoder {
    func encode(_ date: Date) throws -> String {
        date.formatted(.iso8601)
    }

    func decode(_ string: String) throws -> Date {
        if let date = try? Date(string, strategy: .iso8601.year().month().day().time(includingFractionalSeconds: true).timeZone(separator: .omitted)) {
            return date
        }
        if let date = ISO8601DateFormatter.fractional.date(from: string) ?? ISO8601DateFormatter.plain.date(from: string) {
            return date
        }
        throw DecodingError.dataCorrupted(.init(codingPath: [], debugDescription: "Unrecognised date: \(string)"))
    }
}

private extension ISO8601DateFormatter {
    nonisolated(unsafe) static let fractional: ISO8601DateFormatter = {
        let f = ISO8601DateFormatter()
        f.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return f
    }()

    nonisolated(unsafe) static let plain: ISO8601DateFormatter = {
        let f = ISO8601DateFormatter()
        f.formatOptions = [.withInternetDateTime]
        return f
    }()
}
