import SwiftUI

// Platform-specific modifiers so Features builds on iOS, macOS and watchOS.
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

    /// Segmented control on phone/Mac; default wheel/list style on watch.
    func segmentedPickerStyle() -> some View {
        #if os(watchOS)
        self
        #else
        pickerStyle(.segmented)
        #endif
    }

    /// Selectable text where the platform supports it.
    func selectableText() -> some View {
        #if os(watchOS)
        self
        #else
        textSelection(.enabled)
        #endif
    }

    /// Toolbar / inset chrome background (`.bar` is unavailable on watchOS).
    func chromeBackground() -> some View {
        #if os(watchOS)
        background(.ultraThinMaterial)
        #else
        background(.bar)
        #endif
    }
}
