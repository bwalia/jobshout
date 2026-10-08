import Foundation
import JobShoutAPI
import SwiftUI

struct ApprovalsView: View {
    @Environment(AppModel.self) private var app
    @State private var approvals = Resource<[Approval]>()
    @State private var agentNames: [String: String] = [:]

    var body: some View {
        ResourceView(resource: approvals, retry: load) { list in
            List {
                Section {
                    ForEach(list, id: \.id) { approval in
                        NavigationLink(value: ApprovalRoute(id: approval.id)) {
                            VStack(alignment: .leading, spacing: 4) {
                                Text(ToolName.display(approval.toolName)).font(.headline)
                                Text(agentNames[approval.agentId] ?? "Agent")
                                    .font(.subheadline).foregroundStyle(.secondary)
                                Text(approval.requestedAt, format: .relative(presentation: .named))
                                    .font(.caption).foregroundStyle(.secondary)
                            }
                            .padding(.vertical, 2)
                        }
                    }
                } footer: {
                    SyncFooter(loadedAt: approvals.loadedAt)
                }
                if list.isEmpty {
                    ContentUnavailableView("Nothing waiting on you", systemImage: "checkmark.shield",
                                           description: Text("When an agent needs a decision, it appears here and you get a notification."))
                }
            }
            .refreshable { await load() }
        }
        .navigationTitle("Approvals")
        .task(id: app.revision(.approvals)) { await load() }
    }

    private func load() async {
        await approvals.load { try await app.api.approvals(status: "pending") }
        if let agents = try? await app.api.agents() {
            agentNames = Dictionary(agents.map { ($0.id, $0.name) }, uniquingKeysWith: { a, _ in a })
        }
        await app.refreshApprovalCount()
    }
}

struct ApprovalDetailView: View {
    @Environment(AppModel.self) private var app
    @Environment(\.dismiss) private var dismiss
    let approvalID: String
    @State private var approval = Resource<Approval>()
    @State private var agent: Agent?
    @State private var rejecting = false
    @State private var reason = ""
    @State private var deciding = false
    @State private var error: String?

    var body: some View {
        ResourceView(resource: approval, retry: load) { approval in
            List {
                Section {
                    VStack(alignment: .leading, spacing: 8) {
                        Text("\(agent?.name ?? "An agent") wants to")
                            .foregroundStyle(.secondary)
                        Text(ToolName.display(approval.toolName)).font(.title2.bold())
                        StatusPill(status: approval.status)
                    }
                    .padding(.vertical, 4)
                    LabeledContent("Requested", value: approval.requestedAt.formatted(date: .abbreviated, time: .shortened))
                    if let agent { LabeledContent("Agent", value: agent.name) }
                    LabeledContent("Tool", value: approval.toolName)
                }
                if let input = ToolInput.pretty(approval.toolInput) {
                    Section("What it will send") {
                        Text(input)
                            .font(.system(.footnote, design: .monospaced))
                            .selectableText()
                    }
                }
                if let reason = approval.reason, !reason.isEmpty {
                    Section("Reason") { Text(reason) }
                }
                if let error {
                    Section { Label(error, systemImage: "exclamationmark.triangle").foregroundStyle(.red) }
                }
            }
            .safeAreaInset(edge: .bottom) {
                if approval.status == "pending" {
                    HStack(spacing: 12) {
                        Button(role: .destructive) { rejecting = true } label: {
                            Text("Reject").frame(maxWidth: .infinity)
                        }
                        .buttonStyle(.bordered)
                        Button { Task { await decide(approve: true) } } label: {
                            Text("Approve").bold().frame(maxWidth: .infinity)
                        }
                        .buttonStyle(.borderedProminent)
                    }
                    .controlSize(.large)
                    .disabled(deciding)
                    .padding()
                    .chromeBackground()
                }
            }
            .alert("Reject this action?", isPresented: $rejecting) {
                TextField("Reason (optional)", text: $reason)
                Button("Reject", role: .destructive) { Task { await decide(approve: false) } }
                Button("Cancel", role: .cancel) {}
            } message: {
                Text("The agent is told it was rejected and why.")
            }
        }
        .navigationTitle("Approval")
        .inlineTitle()
        .task { await load() }
    }

    private func load() async {
        await approval.load { try await app.api.getApproval(approvalID) }
        if let agentID = approval.value?.agentId {
            agent = try? await app.api.agent(agentID)
        }
    }

    private func decide(approve: Bool) async {
        if approve {
            let ok = await BiometricGate.confirm(reason: "Approve \(ToolName.display(approval.value?.toolName ?? "this action"))")
            guard ok else { return }
        }
        deciding = true
        error = nil
        defer { deciding = false }
        do {
            _ = try await app.api.decide(approvalID: approvalID, approve: approve, reason: approve ? nil : reason)
            await app.refreshApprovalCount()
            dismiss()
        } catch {
            self.error = APIError.from(error).message
        }
    }
}

enum ToolName {
    /// "k8s_apply_manifest" → "K8s apply manifest".
    static func display(_ raw: String) -> String {
        let spaced = raw.replacingOccurrences(of: "_", with: " ").replacingOccurrences(of: ".", with: " ")
        return spaced.prefix(1).uppercased() + spaced.dropFirst()
    }
}

enum ToolInput {
    static func pretty(_ input: Components.Schemas.Approval.ToolInputPayload?) -> String? {
        guard let input else { return nil }
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.prettyPrinted, .sortedKeys, .withoutEscapingSlashes]
        guard let data = try? encoder.encode(input), let s = String(data: data, encoding: .utf8), s != "{}" else { return nil }
        return s
    }
}
