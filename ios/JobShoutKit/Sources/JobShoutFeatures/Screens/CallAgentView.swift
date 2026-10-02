import JobShoutAPI
import SwiftUI

/// The schema-driven launch form. Every agent gets the same screen; the
/// fields come from the server.
struct CallAgentView: View {
    @Environment(AppModel.self) private var app
    @Environment(\.dismiss) private var dismiss
    let agent: Agent
    let onLaunched: (String) -> Void

    @State private var form: LaunchForm?
    @State private var projects: [Project] = []
    @State private var projectID: String?
    @State private var loadError: String?
    @State private var submitError: String?
    @State private var sending = false

    var body: some View {
        NavigationStack {
            Group {
                if let form {
                    formBody(form)
                } else if let loadError {
                    ContentUnavailableView("Couldn't load the form", systemImage: "exclamationmark.triangle", description: Text(loadError))
                } else {
                    ProgressView()
                }
            }
            .navigationTitle("Call \(agent.name)")
            .inlineTitle()
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Run now") { Task { await run() } }
                        .bold()
                        .disabled(form == nil || projectID == nil || sending)
                }
            }
            .task { await load() }
        }
        .interactiveDismissDisabled(sending)
    }

    @ViewBuilder
    private func formBody(_ form: LaunchForm) -> some View {
        let errors = form.showErrors ? form.fieldErrors() : [:]
        Form {
            if let hint = form.schema.hint, !hint.isEmpty {
                Section { Text(hint).foregroundStyle(.secondary) }
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
                } footer: {
                    Text("Agent work is filed under a project. This creates one called \u{201C}My work\u{201D}; rename it on the web any time.")
                }
            }
            ForEach(groups(form.fields), id: \.title) { group in
                Section(group.title) {
                    ForEach(group.fields, id: \.key) { field in
                        SchemaFieldView(field: field, form: form, error: errors[field.key])
                    }
                }
            }
            if form.showErrors {
                ForEach(form.groupErrors(), id: \.self) { message in
                    Label(message, systemImage: "exclamationmark.circle").foregroundStyle(.red)
                }
            }
            if let submitError {
                Section { Label(submitError, systemImage: "exclamationmark.triangle").foregroundStyle(.red) }
            }
            if sending {
                Section { HStack { ProgressView(); Text("Starting…") } }
            }
        }
    }

    private struct FieldGroup { let title: String; let fields: [AgentSchemaField] }

    /// Keeps the schema's order; consecutive fields sharing a group title
    /// share a section.
    private func groups(_ fields: [AgentSchemaField]) -> [FieldGroup] {
        var out: [FieldGroup] = []
        for field in fields {
            let title = field.group ?? ""
            if let last = out.last, last.title == title {
                out[out.count - 1] = FieldGroup(title: title, fields: last.fields + [field])
            } else {
                out.append(FieldGroup(title: title, fields: [field]))
            }
        }
        return out
    }

    private func load() async {
        do {
            async let schema = app.api.launchSchema(for: agent)
            async let projectList = app.api.projects()
            let (s, p) = try await (schema, projectList)
            projects = p
            projectID = p.first?.id
            form = LaunchForm(schema: s)
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

    private func run() async {
        guard let form, let values = form.submission() else { return }
        sending = true
        submitError = nil
        defer { sending = false }
        do {
            let result = try await app.api.launch(agentID: agent.id, projectID: projectID, values: values)
            if let taskID = result.task?.id {
                onLaunched(taskID)
            } else {
                dismiss()
            }
        } catch {
            submitError = APIError.from(error).message
        }
    }
}

struct SchemaFieldView: View {
    let field: AgentSchemaField
    @Bindable var form: LaunchForm
    let error: String?

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            if showsLabelAbove {
                Text(label).font(.subheadline.weight(.medium))
            }
            input
            if let help = field.help, !help.isEmpty {
                Text(help).font(.footnote).foregroundStyle(.secondary)
            }
            if let error {
                Text(error).font(.footnote).foregroundStyle(.red)
            }
        }
    }

    private var text: Binding<String> {
        Binding(get: { form.binding(field.key) }, set: { form.set(field.key, $0) })
    }

    private var label: String { field.required ? "\(field.label) *" : field.label }

    /// Toggles and pickers carry their own label; text inputs only show a
    /// placeholder, which disappears once you type.
    private var showsLabelAbove: Bool {
        switch field.kind {
        case .checkbox, .select: false
        default: true
        }
    }

    @ViewBuilder
    private var input: some View {
        switch field.kind {
        case .checkbox:
            Toggle(field.label, isOn: Binding(
                get: { form.binding(field.key) == "true" },
                set: { form.set(field.key, $0 ? "true" : "false") }))
        case .select:
            Picker(label, selection: text) {
                if !field.required { Text("None").tag("") }
                ForEach(field.options ?? [], id: \.value) { Text($0.label).tag($0.value) }
            }
        case .textarea:
            TextField(label, text: text, prompt: Text(field.placeholder ?? ""), axis: .vertical)
                .lineLimit(3...8)
        case .number:
            TextField(label, text: text, prompt: Text(field.placeholder ?? ""))
                .numberField()
        case .tags:
            TextField(label, text: text, prompt: Text(field.placeholder ?? "Comma-separated"))
                .plainField()
        case .text:
            TextField(label, text: text, prompt: Text(field.placeholder ?? ""))
        }
    }
}
