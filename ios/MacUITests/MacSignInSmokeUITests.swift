import XCTest

/// Automated Mac smoke against int: launch JobShoutMac, confirm the sign-in
/// screen talks to Integration and offers Google. Optional email sign-in when
/// credentials are provided (TEST_RUNNER_JOBSHOUT_UITEST_*).
final class MacSignInSmokeUITests: XCTestCase {
    override func setUp() {
        continueAfterFailure = false
    }

    func testSignInScreenAgainstInt() throws {
        let app = XCUIApplication()
        app.launchEnvironment["JOBSHOUT_ENVIRONMENT"] = "int"
        app.launch()

        // Either signed out (sign-in form) or already signed in from a prior run.
        let email = app.textFields["email"]
        let sidebar = app.descendants(matching: .any)["mac-sidebar"]
        let signedOut = email.waitForExistence(timeout: 12)
        let signedIn = sidebar.waitForExistence(timeout: signedOut ? 1 : 12)
        XCTAssertTrue(signedOut || signedIn, "expected sign-in form or Mac sidebar")

        if signedOut {
            let connected = app.staticTexts.matching(NSPredicate(format: "label CONTAINS %@", "Integration")).firstMatch
            XCTAssertTrue(connected.waitForExistence(timeout: 5), "footer should say Connected to Integration")

            let google = app.buttons["google-sign-in"]
            XCTAssertTrue(google.waitForExistence(timeout: 10), "Google sign-in should be offered on int")
            snapshot(app, "mac-sign-in-int")
        } else {
            snapshot(app, "mac-already-signed-in")
        }
    }

    func testEmailSignInListsAgents() throws {
        let env = ProcessInfo.processInfo.environment
        guard let email = env["JOBSHOUT_UITEST_EMAIL"], let password = env["JOBSHOUT_UITEST_PASSWORD"] else {
            throw XCTSkip("Set JOBSHOUT_UITEST_EMAIL and JOBSHOUT_UITEST_PASSWORD to sign in on int.")
        }

        let app = XCUIApplication()
        app.launchEnvironment["JOBSHOUT_ENVIRONMENT"] = "int"
        app.launch()

        if app.textFields["email"].waitForExistence(timeout: 8) {
            let emailField = app.textFields["email"]
            emailField.click()
            emailField.typeText(email)
            let passwordField = app.secureTextFields["password"]
            passwordField.click()
            passwordField.typeText(password)
            app.buttons["submit"].click()
        }

        let agents = app.descendants(matching: .any)["sidebar-agents"]
        XCTAssertTrue(agents.waitForExistence(timeout: 20), "Mac sidebar Agents after sign-in")
        agents.click()

        let agentRow = app.buttons.matching(identifier: "agent-row").firstMatch
        XCTAssertTrue(agentRow.waitForExistence(timeout: 15), "agents should load from int")
        snapshot(app, "mac-agents-int")
    }

    private func snapshot(_ app: XCUIApplication, _ name: String) {
        let attachment = XCTAttachment(screenshot: app.screenshot())
        attachment.name = name
        attachment.lifetime = .keepAlways
        add(attachment)
    }
}
