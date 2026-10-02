import JobShoutAPI
import SwiftUI

/// The command centre: what's running, what needs you, what just finished.
struct HomeView: View {
    @Environment(AppModel.self) private var app
    @State private var me: User?
    @State private var summary = Resource<Summary>()

    struct Summary {
        var running: [WorkTask]
        var review: [WorkTask]
        var recent: [WorkTask]
        var totalAgents: Int
    }

    var body: some View {
        ResourceView(resource: summary, retry: load) { s in
            List {
                Section {
                    LazyVGrid(columns: [GridItem(.flexible()), GridItem(.flexible())], spacing: 12) {
                        StatTile(value: s.running.count, label: "Running", symbol: "circle.dotted.circle", tint: .blue)
                        StatTile(value: app.pendingApprovals, label: "Need approval", symbol: "checkmark.shield", tint: .orange)
                        StatTile(value: s.review.count, label: "In review", symbol: "eye", tint: .purple)
                        StatTile(value: s.totalAgents, label: "Agents", symbol: "sparkles", tint: .green)
                    }
                    .listRowInsets(EdgeInsets())
                    .listRowBackground(Color.clear)
                } header: {
                    Text(greeting).font(.title2.bold()).foregroundStyle(.primary).textCase(nil)
                } footer: {
                    SyncFooter(loadedAt: summary.loadedAt)
                }

                if !s.running.isEmpty {
                    Section("Running now") {
                        ForEach(s.running.prefix(5), id: \.id) { task in
                            NavigationLink(value: TaskRoute(id: task.id)) { TaskRow(task: task) }
                        }
                    }
                }
                Section("Recent work") {
                    if s.recent.isEmpty {
                        Text("Nothing yet. Call an agent from the Agents tab.").foregroundStyle(.secondary)
                    }
                    ForEach(s.recent.prefix(8), id: \.id) { task in
                        NavigationLink(value: TaskRoute(id: task.id)) { TaskRow(task: task) }
                    }
                }
            }
            .refreshable { await load() }
        }
        .navigationTitle("Home")
        .task(id: app.revision(.work)) { await load() }
        .task { me = try? await app.api.me() }
    }

    private var greeting: String {
        let hour = Calendar.current.component(.hour, from: .now)
        let part = hour < 12 ? "Good morning" : hour < 18 ? "Good afternoon" : "Good evening"
        guard let first = me?.fullName.split(separator: " ").first else { return part }
        return "\(part), \(first)"
    }

    private func load() async {
        await summary.load {
            async let running = app.api.tasks(status: "in_progress", perPage: 20)
            async let review = app.api.tasks(status: "review", perPage: 20)
            async let recent = app.api.tasks(perPage: 20)
            async let agents = app.api.agents()
            let (r, rv, rc, a) = try await (running, review, recent, agents)
            return Summary(
                running: r, review: rv,
                recent: rc.sorted { $0.updatedAt > $1.updatedAt },
                totalAgents: a.count)
        }
        await app.refreshApprovalCount()
    }
}

struct StatTile: View {
    let value: Int
    let label: String
    let symbol: String
    let tint: Color
    var caption: String?

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Image(systemName: symbol).foregroundStyle(tint).font(.title3)
            HStack(alignment: .firstTextBaseline, spacing: 4) {
                Text(value, format: .number).font(.title.bold()).contentTransition(.numericText())
                if let caption { Text(caption).font(.caption).foregroundStyle(.secondary) }
            }
            Text(label).font(.subheadline).foregroundStyle(.secondary)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(14)
        .background(.background.secondary, in: .rect(cornerRadius: 16))
        .accessibilityElement(children: .combine)
    }
}
