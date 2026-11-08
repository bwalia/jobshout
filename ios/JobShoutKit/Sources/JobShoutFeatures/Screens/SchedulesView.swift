import JobShoutAPI
import SwiftUI

struct SchedulesView: View {
    @Environment(AppModel.self) private var app
    @State private var schedules = Resource<[Schedule]>()
    @State private var actionError: String?
    @State private var creating = false

    var body: some View {
        ResourceView(resource: schedules, retry: load) { list in
            List {
                if let actionError {
                    Section {
                        Label(actionError, systemImage: "exclamationmark.triangle").foregroundStyle(.red)
                    }
                }
                Section {
                    ForEach(list, id: \.id) { schedule in
                        ScheduleRow(schedule: schedule) { status in
                            await setStatus(schedule.id, status: status)
                        }
                    }
                } footer: {
                    SyncFooter(loadedAt: schedules.loadedAt)
                }
                if list.isEmpty {
                    ContentUnavailableView(
                        "No schedules",
                        systemImage: "clock",
                        description: Text("Create a schedule to run an agent later. Pause and resume anytime."))
                }
            }
            .refreshable { await load() }
        }
        .navigationTitle("Schedules")
        .toolbar {
            ToolbarItem(placement: .primaryAction) {
                Button {
                    creating = true
                } label: {
                    Image(systemName: "plus")
                }
                .accessibilityIdentifier("create-schedule")
            }
        }
        .sheet(isPresented: $creating) {
            CreateScheduleView {
                creating = false
                Task { await load() }
            }
        }
        .task { await load() }
    }

    private func load() async {
        await schedules.load {
            try await app.api.schedules().sorted {
                ($0.nextRunAt ?? .distantPast) > ($1.nextRunAt ?? .distantPast)
            }
        }
    }

    private func setStatus(_ id: String, status: String) async {
        actionError = nil
        do {
            _ = try await app.api.setScheduleStatus(id, status: status)
            await load()
        } catch {
            actionError = APIError.from(error).message
        }
    }
}

struct CreateScheduleView: View {
    @Environment(AppModel.self) private var app
    @Environment(\.dismiss) private var dismiss
    let onCreated: () -> Void

    @State private var agents: [Agent] = []
    @State private var agentID: String?
    @State private var name = ""
    @State private var prompt = ""
    @State private var preset = "every_morning_9am"
    @State private var loadError: String?
    @State private var submitError: String?
    @State private var sending = false

    private let presets: [(id: String, label: String)] = [
        ("every_5m", "Every 5 minutes"),
        ("every_15m", "Every 15 minutes"),
        ("hourly", "Hourly"),
        ("every_morning_9am", "Every morning 9am"),
        ("every_morning_10am", "Every morning 10am"),
        ("every_midnight", "Every midnight"),
        ("weekdays_9am", "Weekdays 9am"),
        ("weekly_monday_9am", "Mondays 9am"),
    ]

    var body: some View {
        NavigationStack {
            Form {
                if let loadError {
                    Section { Label(loadError, systemImage: "exclamationmark.triangle").foregroundStyle(.red) }
                }
                Section("Agent") {
                    Picker("Agent", selection: $agentID) {
                        Text("Select…").tag(Optional<String>.none)
                        ForEach(agents, id: \.id) { Text($0.name).tag(Optional($0.id)) }
                    }
                }
                Section("Schedule") {
                    TextField("Name", text: $name)
                    Picker("When", selection: $preset) {
                        ForEach(presets, id: \.id) { Text($0.label).tag($0.id) }
                    }
                    TextField("Prompt", text: $prompt, axis: .vertical)
                        .lineLimit(3...8)
                }
                if let submitError {
                    Section { Label(submitError, systemImage: "exclamationmark.triangle").foregroundStyle(.red) }
                }
            }
            .navigationTitle("New schedule")
            .inlineTitle()
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Create") { Task { await create() } }
                        .bold()
                        .disabled(!canSubmit || sending)
                }
            }
            .task { await load() }
        }
        .interactiveDismissDisabled(sending)
    }

    private var canSubmit: Bool {
        agentID != nil
            && name.trimmingCharacters(in: .whitespacesAndNewlines).count >= 2
            && !prompt.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
    }

    private func load() async {
        do {
            agents = try await app.api.agents()
            if agentID == nil { agentID = agents.first?.id }
            if name.isEmpty, let agent = agents.first(where: { $0.id == agentID }) {
                name = "\(agent.name) schedule"
            }
        } catch {
            loadError = APIError.from(error).message
        }
    }

    private func create() async {
        guard let agentID else { return }
        sending = true
        submitError = nil
        defer { sending = false }
        do {
            _ = try await app.api.createSchedule(
                name: name.trimmingCharacters(in: .whitespacesAndNewlines),
                agentID: agentID,
                prompt: prompt.trimmingCharacters(in: .whitespacesAndNewlines),
                preset: preset
            )
            onCreated()
        } catch {
            submitError = APIError.from(error).message
        }
    }
}

private struct ScheduleRow: View {
    let schedule: Schedule
    let onStatus: (String) async -> Void
    @State private var busy = false

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack {
                VStack(alignment: .leading, spacing: 2) {
                    Text(schedule.name).font(.headline)
                    Text(subtitle).font(.caption).foregroundStyle(.secondary)
                }
                Spacer()
                StatusPill(status: schedule.status)
            }
            HStack {
                if schedule.status == "active" {
                    Button("Pause") { Task { await act("paused") } }
                        .disabled(busy)
                } else if schedule.status == "paused" {
                    Button("Resume") { Task { await act("active") } }
                        .disabled(busy)
                }
                Spacer()
                if let next = schedule.nextRunAt {
                    Text("Next \(next.formatted(.relative(presentation: .named)))")
                        .font(.caption2).foregroundStyle(.secondary)
                }
            }
        }
        .padding(.vertical, 4)
    }

    private var subtitle: String {
        var parts = [schedule.scheduleType]
        if let cron = schedule.cronExpression, !cron.isEmpty { parts.append(cron) }
        if let preset = schedule.schedulePreset, !preset.isEmpty { parts.append(preset) }
        return parts.joined(separator: " · ")
    }

    private func act(_ status: String) async {
        busy = true
        defer { busy = false }
        await onStatus(status)
    }
}
