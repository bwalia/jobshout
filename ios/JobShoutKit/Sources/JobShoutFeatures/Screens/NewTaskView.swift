import JobShoutAPI
import SwiftUI

/// Create a board task assigned to an agent without starting a run.
struct NewTaskView: View {
    @Environment(AppModel.self) private var app
    @Environment(\.dismiss) private var dismiss
    let agent: Agent
    let onCreated: (String) -> Void

    @State private var title = ""
    @State private var details = ""
    @State private var projects: [Project] = []
    @State private var projectID: String?
    @State private var loadError: String?
    @State private var submitError: String?
    @State private var sending = false

    var body: some View {
        NavigationStack {
            Form {
                if let loadError {
                    Section { Label(loadError, systemImage: "exclamationmark.triangle").foregroundStyle(.red) }
                }
                if projects.count > 1 {
                    Section("Project") {
                        Picker("Project", selection: $projectID) {
                            ForEach(projects, id: \.id) { Text($0.name).tag(Optional($0.id)) }
                        }
                    }
                } else if projects.isEmpty {
                    Section {
                        Button {
                            Task { await createFirstProject() }
                        } label: {
                            Label("Create a project for this work", systemImage: "folder.badge.plus")
                        }
                    }
                }
                Section("Task") {
                    TextField("Title", text: $title)
                    TextField("Description", text: $details, axis: .vertical)
                        .lineLimit(3...8)
                }
                if let submitError {
                    Section { Label(submitError, systemImage: "exclamationmark.triangle").foregroundStyle(.red) }
                }
            }
            .navigationTitle("New task")
            .inlineTitle()
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Create") { Task { await create() } }
                        .bold()
                        .disabled(title.trimmingCharacters(in: .whitespacesAndNewlines).count < 2 || projectID == nil || sending)
                }
            }
            .task { await load() }
        }
        .interactiveDismissDisabled(sending)
    }

    private func load() async {
        do {
            projects = try await app.api.projects()
            projectID = projects.first?.id
        } catch {
            loadError = APIError.from(error).message
        }
    }

    private func createFirstProject() async {
        do {
            let project = try await app.api.createProject(name: "My work")
            projects = [project]
            projectID = project.id
        } catch {
            submitError = APIError.from(error).message
        }
    }

    private func create() async {
        guard let projectID else { return }
        sending = true
        submitError = nil
        defer { sending = false }
        do {
            let task = try await app.api.createTask(
                projectID: projectID,
                title: title.trimmingCharacters(in: .whitespacesAndNewlines),
                description: details.nilIfBlank,
                agentID: agent.id,
                status: "backlog"
            )
            onCreated(task.id)
        } catch {
            submitError = APIError.from(error).message
        }
    }
}
