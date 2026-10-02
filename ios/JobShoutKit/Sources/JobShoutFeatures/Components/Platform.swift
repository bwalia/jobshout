import SwiftUI

// iOS-only modifiers, no-ops on macOS so the package also builds and tests
// there.
extension View {
    func inlineTitle() -> some View {
        #if os(iOS)
        navigationBarTitleDisplayMode(.inline)
        #else
        self
        #endif
    }

    func emailField() -> some View {
        #if os(iOS)
        textInputAutocapitalization(.never).keyboardType(.emailAddress).autocorrectionDisabled()
        #else
        autocorrectionDisabled()
        #endif
    }

    func numberField() -> some View {
        #if os(iOS)
        keyboardType(.numberPad)
        #else
        self
        #endif
    }

    func plainField() -> some View {
        #if os(iOS)
        textInputAutocapitalization(.never).autocorrectionDisabled()
        #else
        autocorrectionDisabled()
        #endif
    }
}
