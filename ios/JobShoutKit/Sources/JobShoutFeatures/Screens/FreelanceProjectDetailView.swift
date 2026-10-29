import SwiftUI

struct FreelanceProjectDetailView: View {
    let projectID: String

    private var project: FreelanceProject? { FreelanceCatalog.project(id: projectID) }

    var body: some View {
        Group {
            if let project {
                content(project)
            } else {
                ContentUnavailableView("Project not found", systemImage: "briefcase")
            }
        }
        .navigationTitle("Project")
        .inlineTitle()
    }

    @ViewBuilder
    private func content(_ project: FreelanceProject) -> some View {
        List {
            Section {
                Text(project.title).font(.title2.bold())
                Text(project.summary).foregroundStyle(.secondary)
                HStack(spacing: 12) {
                    Label(project.budget.display, systemImage: "dollarsign.circle")
                    Label(project.duration, systemImage: "clock")
                }
                .font(.subheadline)
                Text("\(project.bidCount) bids · Posted \(project.posted)")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }

            Section("About") {
                Text(project.description)
            }

            Section("Deliverables") {
                ForEach(project.deliverables, id: \.self) { item in
                    Label(item, systemImage: "checkmark.circle")
                }
            }

            Section("Skills") {
                FlowSkills(skills: project.skills)
            }

            Section("Client") {
                LabeledContent("Name", value: project.client.name)
                LabeledContent("Location", value: project.client.location)
                LabeledContent("Projects posted", value: "\(project.client.projectsPosted)")
                LabeledContent("Hire rate", value: project.client.hireRate)
            }

            Section {
                NavigationLink(value: FreelanceBidRoute(projectID: project.id)) {
                    Label("Submit a bid", systemImage: "paperplane.fill")
                        .font(.headline)
                }
                .accessibilityIdentifier("submit-bid")
            }
        }
    }
}

private struct FlowSkills: View {
    let skills: [String]

    var body: some View {
        FlexibleWrap(skills)
    }
}

/// Simple wrapping chip row without a third-party layout package.
private struct FlexibleWrap: View {
    let skills: [String]
    init(_ skills: [String]) { self.skills = skills }

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            ForEach(rows, id: \.self) { row in
                HStack(spacing: 6) {
                    ForEach(row, id: \.self) { skill in
                        Text(skill)
                            .font(.caption.weight(.medium))
                            .padding(.horizontal, 10)
                            .padding(.vertical, 5)
                            .background(.quaternary.opacity(0.5), in: Capsule())
                    }
                    Spacer(minLength: 0)
                }
            }
        }
    }

    private var rows: [[String]] {
        var out: [[String]] = []
        var current: [String] = []
        var width = 0
        for skill in skills {
            let w = skill.count + 2
            if width + w > 36, !current.isEmpty {
                out.append(current)
                current = [skill]
                width = w
            } else {
                current.append(skill)
                width += w
            }
        }
        if !current.isEmpty { out.append(current) }
        return out
    }
}
