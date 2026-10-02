import SwiftUI

/// One vocabulary for every status string the platform sends (tasks, runs,
/// executions, approvals, agents). Unknown values render neutrally.
struct StatusPill: View {
    let status: String

    var body: some View {
        let style = StatusStyle(status)
        Label(style.label, systemImage: style.symbol)
            .font(.caption.weight(.semibold))
            .labelStyle(.titleAndIcon)
            .padding(.horizontal, 8)
            .padding(.vertical, 3)
            .foregroundStyle(style.color)
            .background(style.color.opacity(0.14), in: .capsule)
            .accessibilityLabel("Status: \(style.label)")
    }
}

struct StatusStyle {
    let label: String
    let symbol: String
    let color: Color

    init(_ raw: String) {
        switch raw {
        case "running", "in_progress", "active":
            (label, symbol, color) = (raw == "active" ? "Active" : "Running", "circle.dotted.circle", .blue)
        case "queued", "pending", "todo", "backlog":
            (label, symbol, color) = (raw == "pending" ? "Awaiting approval" : raw == "queued" ? "Queued" : raw == "todo" ? "To do" : "Backlog", "clock", .orange)
        case "review":
            (label, symbol, color) = ("In review", "eye", .purple)
        case "completed", "done", "approved":
            (label, symbol, color) = (raw == "approved" ? "Approved" : "Completed", "checkmark.circle.fill", .green)
        case "failed", "rejected":
            (label, symbol, color) = (raw == "rejected" ? "Rejected" : "Failed", "xmark.octagon.fill", .red)
        case "cancelled":
            (label, symbol, color) = ("Cancelled", "slash.circle", .secondary)
        case "idle":
            (label, symbol, color) = ("Idle", "moon.zzz", .secondary)
        case "paused":
            (label, symbol, color) = ("Paused", "pause.circle", .orange)
        case "offline":
            (label, symbol, color) = ("Offline", "wifi.slash", .secondary)
        default:
            (label, symbol, color) = (raw.replacingOccurrences(of: "_", with: " ").capitalized, "circle", .secondary)
        }
    }
}

extension String {
    /// Statuses where the work is still moving and worth polling.
    var isInFlight: Bool { ["running", "queued", "in_progress", "pending"].contains(self) }
}
