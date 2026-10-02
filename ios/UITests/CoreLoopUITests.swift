import XCTest

/// The phase-1 loop against a real server: sign in, find an agent, call it,
/// see the work. Needs a running API and an account on it, so it skips unless
/// told where:
///
///   JOBSHOUT_UITEST_EMAIL, JOBSHOUT_UITEST_PASSWORD   an existing account
///   JOBSHOUT_UITEST_PORT                              local API port (8190)
///
/// e.g. TEST_RUNNER_JOBSHOUT_UITEST_EMAIL=… xcodebuild test -only-testing:JobShoutUITests
final class CoreLoopUITests: XCTestCase {
    override func setUp() {
        continueAfterFailure = false
    }

    func testSignInCallAgentAndSeeWork() throws {
        let env = ProcessInfo.processInfo.environment
        guard let email = env["JOBSHOUT_UITEST_EMAIL"], let password = env["JOBSHOUT_UITEST_PASSWORD"] else {
            throw XCTSkip("Set JOBSHOUT_UITEST_EMAIL and JOBSHOUT_UITEST_PASSWORD to run against a server.")
        }
        let app = XCUIApplication()
        app.launchEnvironment["JOBSHOUT_ENVIRONMENT"] = "local"
        app.launchEnvironment["JOBSHOUT_LOCAL_PORT"] = env["JOBSHOUT_UITEST_PORT"] ?? "8190"
        app.launch()

        // A previous run may have left the app signed in.
        if app.textFields["email"].waitForExistence(timeout: 5) {
            app.textFields["email"].tap()
            app.textFields["email"].typeText(email)
            app.secureTextFields["password"].tap()
            app.secureTextFields["password"].typeText(password)
            app.buttons["submit"].tap()
        }

        XCTAssertTrue(app.tabBars.buttons["Home"].waitForExistence(timeout: 15), "should land on Home")
        snapshot(app, "home")

        app.tabBars.buttons["Agents"].tap()
        let agent = app.buttons.matching(identifier: "agent-row").firstMatch
        XCTAssertTrue(agent.waitForExistence(timeout: 10), "agents should load")
        snapshot(app, "agents")
        agent.tap()

        XCTAssertTrue(app.buttons["Call agent"].waitForExistence(timeout: 10))
        snapshot(app, "agent")
        app.buttons["Call agent"].tap()

        XCTAssertTrue(app.navigationBars.buttons["Run now"].waitForExistence(timeout: 10), "launch form should load")
        let createProject = app.buttons["Create a project for this work"]
        if createProject.waitForExistence(timeout: 2) {
            createProject.tap()
        }
        snapshot(app, "call-agent")

        app.buttons["Cancel"].tap()
        app.tabBars.buttons["Work"].tap()
        XCTAssertTrue(app.segmentedControls.buttons["All"].waitForExistence(timeout: 5))
        app.segmentedControls.buttons["All"].tap()
        snapshot(app, "work")
        let task = app.buttons.matching(identifier: "task-row").firstMatch
        if task.waitForExistence(timeout: 5) {
            task.tap()
            XCTAssertTrue(app.staticTexts["Runs"].waitForExistence(timeout: 10), "task detail should load")
            snapshot(app, "task")
            app.navigationBars.buttons.element(boundBy: 0).tap()
        }

        app.tabBars.buttons["Approvals"].tap()
        snapshot(app, "approvals")
        app.tabBars.buttons["Me"].tap()
        XCTAssertTrue(app.staticTexts["This device"].waitForExistence(timeout: 10), "this device should be listed")
        snapshot(app, "me")
    }

    private func snapshot(_ app: XCUIApplication, _ name: String) {
        let attachment = XCTAttachment(screenshot: app.screenshot())
        attachment.name = name
        attachment.lifetime = .keepAlways
        add(attachment)
    }
}
