import Foundation
import Security

/// What survives an app restart. Lives only in the Keychain.
public struct StoredSession: Codable, Equatable, Sendable {
    public var accessToken: String
    public var refreshToken: String
    public var deviceID: String?
    public var userID: String

    public init(accessToken: String, refreshToken: String, deviceID: String?, userID: String) {
        self.accessToken = accessToken
        self.refreshToken = refreshToken
        self.deviceID = deviceID
        self.userID = userID
    }
}

public protocol TokenStore: Sendable {
    func load() throws -> StoredSession?
    func save(_ session: StoredSession) throws
    func clear() throws
}

/// Keychain-backed store: one generic-password item per environment,
/// readable after first unlock and never synced or migrated to another
/// device (`ThisDeviceOnly`), because the session is bound to this install.
public struct KeychainTokenStore: TokenStore {
    let service: String
    let account: String
    let accessGroup: String?

    public init(account: String, service: String = "com.jobshout.session", accessGroup: String? = nil) {
        self.service = service
        self.account = account
        self.accessGroup = accessGroup
    }

    private var baseQuery: [String: Any] {
        var q: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account,
            kSecUseDataProtectionKeychain as String: true,
        ]
        if let accessGroup { q[kSecAttrAccessGroup as String] = accessGroup }
        return q
    }

    public func load() throws -> StoredSession? {
        var query = baseQuery
        query[kSecReturnData as String] = true
        query[kSecMatchLimit as String] = kSecMatchLimitOne
        var out: CFTypeRef?
        let status = SecItemCopyMatching(query as CFDictionary, &out)
        switch status {
        case errSecSuccess:
            guard let data = out as? Data else { return nil }
            return try JSONDecoder().decode(StoredSession.self, from: data)
        case errSecItemNotFound:
            return nil
        default:
            throw KeychainError(status: status)
        }
    }

    public func save(_ session: StoredSession) throws {
        let data = try JSONEncoder().encode(session)
        let attributes: [String: Any] = [
            kSecValueData as String: data,
            kSecAttrAccessible as String: kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly,
        ]
        let status = SecItemUpdate(baseQuery as CFDictionary, attributes as CFDictionary)
        if status == errSecItemNotFound {
            var add = baseQuery
            add.merge(attributes) { $1 }
            let addStatus = SecItemAdd(add as CFDictionary, nil)
            guard addStatus == errSecSuccess else { throw KeychainError(status: addStatus) }
        } else if status != errSecSuccess {
            throw KeychainError(status: status)
        }
    }

    public func clear() throws {
        let status = SecItemDelete(baseQuery as CFDictionary)
        guard status == errSecSuccess || status == errSecItemNotFound else {
            throw KeychainError(status: status)
        }
    }
}

public struct KeychainError: LocalizedError, CustomStringConvertible {
    public let status: OSStatus

    public var description: String {
        (SecCopyErrorMessageString(status, nil) as String?) ?? "Keychain error \(status)"
    }

    /// errSecMissingEntitlement (-34018) means an unsigned build: sign the
    /// app (ad-hoc is enough on the simulator).
    public var errorDescription: String? {
        "Couldn't store your sign-in securely on this device (\(status))."
    }
}

/// For tests and previews.
public final class InMemoryTokenStore: TokenStore, @unchecked Sendable {
    private let lock = NSLock()
    private var session: StoredSession?

    public init(_ session: StoredSession? = nil) {
        self.session = session
    }

    public func load() throws -> StoredSession? { lock.withLock { session } }
    public func save(_ session: StoredSession) throws { lock.withLock { self.session = session } }
    public func clear() throws { lock.withLock { session = nil } }
}
