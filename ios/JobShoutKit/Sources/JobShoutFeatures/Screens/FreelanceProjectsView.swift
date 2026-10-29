import SwiftUI

/// Browse Upwork-style freelance projects (jobshout.com marketplace).
struct FreelanceProjectsView: View {
    @State private var search = ""

    private var shown: [FreelanceProject] {
        let q = search.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !q.isEmpty else { return FreelanceCatalog.projects }
        return FreelanceCatalog.projects.filter {
            $0.title.localizedCaseInsensitiveContains(q)
                || $0.summary.localizedCaseInsensitiveContains(q)
                || $0.skills.contains { $0.localizedCaseInsensitiveContains(q) }
        }
    }

    var body: some View {
        List {
            Section {
                VStack(alignment: .leading, spacing: 6) {
                    Text("Find freelance projects worth shouting about")
                        .font(.title3.bold())
                    Text("Browse short-term projects, submit bids, and work with clients on flexible terms.")
                        .font(.subheadline)
                        .foregroundStyle(.secondary)
                }
                .padding(.vertical, 4)
                .listRowBackground(Color.clear)
            }

            Section {
                ForEach(shown) { project in
                    NavigationLink(value: FreelanceProjectRoute(id: project.id)) {
                        FreelanceProjectRow(project: project)
                    }
                    .accessibilityIdentifier("freelance-project-row")
                }
                if shown.isEmpty {
                    ContentUnavailableView.search(text: search)
                }
            } footer: {
                Text("Same catalog as jobshout.com /projects. Live API wiring follows.")
            }
        }
        .navigationTitle("Projects")
        .searchable(text: $search, prompt: "React, design, writing…")
    }
}

private struct FreelanceProjectRow: View {
    let project: FreelanceProject

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack {
                Text(project.kind.label.capitalized)
                    .font(.caption.weight(.semibold))
                    .foregroundStyle(.secondary)
                Spacer()
                Text("\(project.bidCount) bids")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
            Text(project.title).font(.headline).lineLimit(2)
            Text(project.summary).font(.subheadline).foregroundStyle(.secondary).lineLimit(2)
            HStack {
                Text(project.budget.display).font(.subheadline.weight(.semibold))
                Text("·").foregroundStyle(.tertiary)
                Text(project.posted).font(.caption).foregroundStyle(.secondary)
            }
            ScrollView(.horizontal, showsIndicators: false) {
                HStack(spacing: 6) {
                    ForEach(project.skills.prefix(4), id: \.self) { skill in
                        Text(skill)
                            .font(.caption2.weight(.medium))
                            .padding(.horizontal, 8)
                            .padding(.vertical, 4)
                            .background(.quaternary.opacity(0.5), in: Capsule())
                    }
                }
            }
        }
        .padding(.vertical, 4)
    }
}
