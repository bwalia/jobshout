import JobShoutLive
import SwiftUI

/// "Updated 2 min ago · Live". Honest about freshness: never claims live
/// status when the socket is down.
struct SyncFooter: View {
    @Environment(AppModel.self) private var app
    let loadedAt: Date?

    var body: some View {
        HStack(spacing: 6) {
            Circle()
                .fill(app.liveState == .connected ? Color.green : Color.secondary)
                .frame(width: 7, height: 7)
                .accessibilityHidden(true)
            Text(text)
        }
        .font(.caption)
        .foregroundStyle(.secondary)
        .accessibilityElement(children: .combine)
    }

    private var text: String {
        let live = switch app.liveState {
        case .connected: "Live"
        case .connecting: "Connecting…"
        case .disconnected: "Not live"
        }
        guard let loadedAt else { return live }
        return "Updated \(loadedAt.formatted(.relative(presentation: .named))) · \(live)"
    }
}
