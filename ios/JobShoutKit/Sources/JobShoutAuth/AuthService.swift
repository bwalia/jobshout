import CryptoKit
import Foundation
import JobShoutAPI
import JobShoutCore

/// Sign-in, sign-out and token refresh against the unauthenticated client.
public struct AuthService: Sendable {
    let client: Client
    let device: DeviceDescriptor

    public init(client: Client, device: DeviceDescriptor) {
        self.client = client
        self.device = device
    }

    public func login(email: String, password: String, knownDeviceID: String?) async throws -> StoredSession {
        let out = try await client.login(body: .json(.init(
            email: email, password: password, device: device.info(knownID: knownDeviceID))))
        switch out {
        case .ok(let ok): return try Self.session(from: ok.body.json)
        case .unauthorized(let r): throw APIError.server(401, try? r.body.json)
        case .undocumented(let status, _): throw APIError.undocumented(status)
        }
    }

    public func register(email: String, password: String, fullName: String, orgName: String, knownDeviceID: String?) async throws -> StoredSession {
        let out = try await client.register(body: .json(.init(
            email: email, password: password, fullName: fullName, orgName: orgName,
            device: device.info(knownID: knownDeviceID))))
        switch out {
        case .created(let created): return try Self.session(from: created.body.json)
        case .conflict(let r): throw APIError.server(409, try? r.body.json)
        case .undocumented(let status, _): throw APIError.undocumented(status)
        }
    }

    public func appleEnabled() async -> Bool {
        guard case .ok(let ok) = try? await client.appleStatus() else { return false }
        return (try? ok.body.json.enabled) ?? false
    }

    /// A one-time nonce from the server. Give Apple `AppleNonce.hash(raw)`.
    public func appleNonce() async throws -> String {
        switch try await client.appleNonce() {
        case .ok(let ok): return try ok.body.json.nonce
        case .serviceUnavailable(let r): throw APIError.server(503, try? r.body.json)
        case .undocumented(let status, _): throw APIError.undocumented(status)
        }
    }

    public func signInWithApple(identityToken: String, rawNonce: String, fullName: String?, knownDeviceID: String?) async throws -> StoredSession {
        let out = try await client.signInWithApple(body: .json(.init(
            identityToken: identityToken, nonce: rawNonce, fullName: fullName,
            device: device.info(knownID: knownDeviceID))))
        switch out {
        case .ok(let ok): return try Self.session(from: ok.body.json)
        case .unauthorized(let r): throw APIError.server(401, try? r.body.json)
        case .unprocessableContent(let r): throw APIError.server(422, try? r.body.json)
        case .serviceUnavailable(let r): throw APIError.server(503, try? r.body.json)
        case .undocumented(let status, _): throw APIError.undocumented(status)
        }
    }

    public func refresh(_ refreshToken: String) async throws -> SessionManager.RefreshOutcome {
        switch try await client.refresh(body: .json(.init(refreshToken: refreshToken))) {
        case .ok(let ok): return .renewed(try Self.session(from: ok.body.json))
        case .unauthorized: return .rejected
        case .undocumented(let status, _): throw APIError.undocumented(status)
        }
    }

    /// Revokes this device's session on the server. Best-effort.
    public func logout(refreshToken: String) async {
        _ = try? await client.logout(body: .json(.init(refreshToken: refreshToken)))
    }

    static func session(from r: Components.Schemas.AuthResponse) throws -> StoredSession {
        StoredSession(accessToken: r.accessToken, refreshToken: r.refreshToken, deviceID: r.deviceId, userID: r.user.id)
    }
}

public enum AppleNonce {
    /// SHA-256, lowercase hex: what the server compares the token's nonce
    /// claim against.
    public static func hash(_ raw: String) -> String {
        SHA256.hash(data: Data(raw.utf8)).map { String(format: "%02x", $0) }.joined()
    }
}

/// How this install describes itself at sign-in.
public struct DeviceDescriptor: Sendable {
    public let name: String
    public let appVersion: String

    public init(name: String, appVersion: String) {
        self.name = name
        self.appVersion = appVersion
    }

    public static func current(bundle: Bundle = .main, name: String) -> DeviceDescriptor {
        let version = bundle.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "0"
        let build = bundle.object(forInfoDictionaryKey: "CFBundleVersion") as? String ?? "0"
        return DeviceDescriptor(name: name, appVersion: "\(version) (\(build))")
    }

    func info(knownID: String?) -> Components.Schemas.DeviceInfo {
        .init(id: knownID, platform: .ios, name: name, appVersion: appVersion)
    }
}
