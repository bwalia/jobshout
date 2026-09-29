import Foundation
import Observation

/// The values and validation for one agent's launch form, driven entirely by
/// the server's schema (no per-agent code in the app). Mirrors the web's
/// encoding: checkboxes send "true"/"false", tags send "a, b".
@MainActor @Observable
public final class LaunchForm {
    public let schema: AgentSchema
    public var values: [String: String]
    /// Errors only show after the first submit attempt.
    public private(set) var showErrors = false

    public init(schema: AgentSchema) {
        self.schema = schema
        var initial: [String: String] = [:]
        for field in schema.fields {
            if let d = field._default, !d.isEmpty {
                initial[field.key] = d
            } else if field.kind == .checkbox {
                initial[field.key] = "false"
            }
        }
        values = initial
    }

    public var fields: [AgentSchemaField] { schema.fields }

    public func binding(_ key: String) -> String {
        values[key] ?? ""
    }

    public func set(_ key: String, _ value: String) {
        values[key] = value
    }

    public func fieldErrors() -> [String: String] {
        var out: [String: String] = [:]
        for field in schema.fields {
            let v = (values[field.key] ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
            if field.required, v.isEmpty {
                out[field.key] = "\(field.label) is required."
                continue
            }
            if v.isEmpty { continue }
            switch field.kind {
            case .number:
                guard let n = Int(v) else { out[field.key] = "\(field.label) must be a whole number."; continue }
                if let min = field.min, min > 0, n < min { out[field.key] = "\(field.label) must be at least \(min)." }
            case .select:
                if let options = field.options, !options.isEmpty, !options.contains(where: { $0.value == v }) {
                    out[field.key] = "Choose one of the options for \(field.label)."
                }
            default:
                if let minLength = field.minLength, minLength > 0, v.count < minLength {
                    out[field.key] = "\(field.label) needs at least \(minLength) characters."
                }
            }
        }
        return out
    }

    /// Unmet "fill at least one of" groups, as the question to show.
    public func groupErrors() -> [String] {
        (schema.requireAny ?? []).compactMap { group in
            let filled = group.keys.contains { !(values[$0] ?? "").trimmingCharacters(in: .whitespacesAndNewlines).isEmpty }
            if filled { return nil }
            let labels = group.keys.compactMap { key in schema.fields.first { $0.key == key }?.label }
            return group.question ?? "Fill in one of: \(labels.joined(separator: ", "))."
        }
    }

    public var isValid: Bool { fieldErrors().isEmpty && groupErrors().isEmpty }

    /// Marks errors visible and returns the values to send, or nil if the
    /// form is invalid.
    public func submission() -> [String: String]? {
        showErrors = true
        guard isValid else { return nil }
        var out: [String: String] = [:]
        for field in schema.fields {
            let v = (values[field.key] ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
            if !v.isEmpty { out[field.key] = v }
        }
        return out
    }
}

public enum FieldKind: Equatable, Sendable {
    case text, textarea, number, select, checkbox, tags
}

extension AgentSchemaField {
    /// Unknown types (e.g. "repo") render as text rather than failing.
    public var kind: FieldKind {
        switch _type {
        case "textarea": .textarea
        case "number": .number
        case "select": .select
        case "checkbox": .checkbox
        case "tags": .tags
        default: .text
        }
    }
}

/// "a, b" <-> ["a", "b"], matching the web's tag input.
public enum Tags {
    public static func parse(_ raw: String) -> [String] {
        raw.split(separator: ",").map { $0.trimmingCharacters(in: .whitespaces) }.filter { !$0.isEmpty }
    }

    public static func join(_ tags: [String]) -> String {
        tags.joined(separator: ", ")
    }
}
