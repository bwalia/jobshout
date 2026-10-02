import SwiftUI

/// Typed navigation values, so one stack can hold agents, tasks and
/// approvals without their string ids colliding.
struct AgentRoute: Hashable { let id: String }
struct TaskRoute: Hashable { let id: String }
struct ApprovalRoute: Hashable { let id: String }

extension View {
    /// Every tab can reach every detail screen (Home links to tasks and
    /// approvals, an agent links to the task it just started).
    func detailDestinations() -> some View {
        navigationDestination(for: AgentRoute.self) { AgentDetailView(agentID: $0.id) }
            .navigationDestination(for: TaskRoute.self) { TaskDetailView(taskID: $0.id) }
            .navigationDestination(for: ApprovalRoute.self) { ApprovalDetailView(approvalID: $0.id) }
    }
}
