import SwiftUI

#if os(macOS)

/// Mac signed-in chrome: sidebar groups match `web/nextjs/lib/nav.ts`.
struct MacShellView: View {
    @Environment(AppModel.self) private var app
    @State private var selection: MacDestination? = .home

    var body: some View {
        NavigationSplitView {
            List(selection: $selection) {
                Section {
                    Label("Home", systemImage: "house").tag(MacDestination.home)
                }
                Section("Work") {
                    Label("Tasks", systemImage: "square.stack.3d.up").tag(MacDestination.tasks)
                    Label("Board", systemImage: "rectangle.split.3x1").tag(MacDestination.board)
                }
                Section("Agents") {
                    Label("All agents", systemImage: "sparkles").tag(MacDestination.agents)
                }
                Section("Automation") {
                    Label("Schedules", systemImage: "clock").tag(MacDestination.schedules)
                }
                Section {
                    Label("Approvals", systemImage: "checkmark.shield")
                        .badge(app.pendingApprovals)
                        .tag(MacDestination.approvals)
                    Label("Settings", systemImage: "gearshape").tag(MacDestination.settings)
                }
            }
            .navigationSplitViewColumnWidth(min: 180, ideal: 220, max: 280)
            .listStyle(.sidebar)
        } detail: {
            NavigationStack {
                Group {
                    switch selection ?? .home {
                    case .home: HomeView()
                    case .tasks: WorkView()
                    case .board: TaskBoardView()
                    case .agents: AgentsView()
                    case .schedules: SchedulesView()
                    case .approvals: ApprovalsView()
                    case .settings: MeView()
                    }
                }
                .detailDestinations()
            }
        }
        .navigationSplitViewStyle(.balanced)
    }
}

enum MacDestination: Hashable {
    case home, tasks, board, agents, schedules, approvals, settings
}

#endif
