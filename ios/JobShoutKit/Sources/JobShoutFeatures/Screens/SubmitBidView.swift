import SwiftUI

/// Bid form matching jobshout.com `/projects/[id]/bid` (mock submit until API auth lands).
struct SubmitBidView: View {
    let projectID: String

    @State private var amount = ""
    @State private var hourlyRate = ""
    @State private var estimatedHours = ""
    @State private var proposal = ""
    @State private var delivery = ""
    @State private var error: String?
    @State private var fieldErrors: [String: String] = [:]
    @State private var pending = false
    @State private var submitted = false

    private var project: FreelanceProject? { FreelanceCatalog.project(id: projectID) }

    var body: some View {
        Group {
            if let project {
                if submitted {
                    success(project)
                } else {
                    form(project)
                }
            } else {
                ContentUnavailableView("Project not found", systemImage: "briefcase")
            }
        }
        .navigationTitle("Submit bid")
        .inlineTitle()
    }

    @ViewBuilder
    private func success(_ project: FreelanceProject) -> some View {
        ContentUnavailableView {
            Label("Bid submitted", systemImage: "checkmark.circle.fill")
        } description: {
            Text("Your proposal for \(project.title) is with the client. They can shortlist or message you from their project inbox.")
        } actions: {
            NavigationLink("Back to project", value: FreelanceProjectRoute(id: project.id))
        }
    }

    @ViewBuilder
    private func form(_ project: FreelanceProject) -> some View {
        Form {
            Section {
                Text("Client budget: \(project.budget.display) · \(project.kind.label)")
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
            }

            if project.isHourly {
                Section("Your rate") {
                    TextField("Hourly rate (USD)", text: $hourlyRate)
                        .numberField()
                    if let e = fieldErrors["hourly_rate"] { Text(e).foregroundStyle(.red).font(.caption) }
                    TextField("Estimated hours", text: $estimatedHours)
                        .numberField()
                    if let e = fieldErrors["estimated_hours"] { Text(e).foregroundStyle(.red).font(.caption) }
                }
            } else {
                Section("Your bid") {
                    TextField("Fixed amount (USD)", text: $amount)
                        .numberField()
                    if let e = fieldErrors["amount"] { Text(e).foregroundStyle(.red).font(.caption) }
                }
            }

            Section("Proposal") {
                TextField("How you'll deliver this", text: $proposal, axis: .vertical)
                    .lineLimit(5...12)
                if let e = fieldErrors["proposal"] { Text(e).foregroundStyle(.red).font(.caption) }
                TextField("Delivery timeline", text: $delivery)
                    .plainField()
                if let e = fieldErrors["delivery_timeline"] { Text(e).foregroundStyle(.red).font(.caption) }
            }

            if let error {
                Section { Text(error).foregroundStyle(.red) }
            }

            Section {
                Button {
                    submit(project)
                } label: {
                    if pending { ProgressView() } else { Text("Submit bid").bold() }
                }
                .disabled(pending)
                .accessibilityIdentifier("submit-bid-button")
            }
        }
    }

    private func submit(_ project: FreelanceProject) {
        error = nil
        var next: [String: String] = [:]
        let proposalText = proposal.trimmingCharacters(in: .whitespacesAndNewlines)
        let deliveryText = delivery.trimmingCharacters(in: .whitespacesAndNewlines)
        if proposalText.count < 40 {
            next["proposal"] = "Write at least a short proposal (40+ characters)."
        }
        if deliveryText.isEmpty {
            next["delivery_timeline"] = "How long will this take?"
        }
        if project.isHourly {
            let rate = Double(hourlyRate) ?? 0
            let hours = Double(estimatedHours) ?? 0
            if rate <= 0 { next["hourly_rate"] = "Enter a positive hourly rate." }
            if hours <= 0 { next["estimated_hours"] = "Estimate total hours." }
        } else {
            let value = Double(amount) ?? 0
            if value <= 0 {
                next["amount"] = "Enter your fixed bid amount."
            } else if case .fixed(let min, let max) = project.budget {
                if value < min { next["amount"] = "Below the client’s minimum ($\(Int(min)))." }
                else if value > max * 1.5 {
                    next["amount"] = "Far above the stated budget ($\(Int(max))). Explain why in the proposal if intentional."
                }
            }
        }
        fieldErrors = next
        guard next.isEmpty else {
            error = "Fix the highlighted fields and try again."
            return
        }
        pending = true
        Task {
            try? await Task.sleep(for: .milliseconds(600))
            pending = false
            submitted = true
        }
    }
}
