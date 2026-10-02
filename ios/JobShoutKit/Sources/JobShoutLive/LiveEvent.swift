import Foundation

/// One WebSocket frame. Events are hints to refetch, never state: the payload
/// is decoded loosely and unknown types are kept, not rejected.
public struct LiveEvent: Decodable, Equatable, Sendable {
    public let type: String
    public let payload: [String: JSONValue]
    public let timestamp: Date?

    public init(type: String, payload: [String: JSONValue] = [:], timestamp: Date? = nil) {
        self.type = type
        self.payload = payload
        self.timestamp = timestamp
    }

    enum CodingKeys: String, CodingKey { case type, payload, timestamp }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        type = try c.decode(String.self, forKey: .type)
        payload = (try? c.decode([String: JSONValue].self, forKey: .payload)) ?? [:]
        timestamp = try? c.decode(Date.self, forKey: .timestamp)
    }

    /// What a screen should refetch when this arrives.
    public var topic: Topic {
        switch type {
        case "task.transitioned", "task.assigned", "execution.status_changed": .work
        case "approval.requested", "approval.decided": .approvals
        case "agent.status_changed": .agents
        default: .other
        }
    }

    public enum Topic: Sendable, Hashable { case work, approvals, agents, other }

    public func string(_ key: String) -> String? {
        if case .string(let s) = payload[key] { return s }
        return nil
    }

    static let decoder: JSONDecoder = {
        let d = JSONDecoder()
        d.dateDecodingStrategy = .custom { decoder in
            let raw = try decoder.singleValueContainer().decode(String.self)
            let f = ISO8601DateFormatter()
            f.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
            if let date = f.date(from: raw) { return date }
            f.formatOptions = [.withInternetDateTime]
            if let date = f.date(from: raw) { return date }
            throw DecodingError.dataCorrupted(.init(codingPath: decoder.codingPath, debugDescription: raw))
        }
        return d
    }()

    public static func decode(_ data: Data) -> LiveEvent? {
        try? decoder.decode(LiveEvent.self, from: data)
    }
}

public enum JSONValue: Decodable, Equatable, Sendable {
    case string(String), number(Double), bool(Bool), object([String: JSONValue]), array([JSONValue]), null

    public init(from decoder: Decoder) throws {
        let c = try decoder.singleValueContainer()
        if c.decodeNil() { self = .null }
        else if let v = try? c.decode(Bool.self) { self = .bool(v) }
        else if let v = try? c.decode(Double.self) { self = .number(v) }
        else if let v = try? c.decode(String.self) { self = .string(v) }
        else if let v = try? c.decode([JSONValue].self) { self = .array(v) }
        else { self = .object(try c.decode([String: JSONValue].self)) }
    }
}
