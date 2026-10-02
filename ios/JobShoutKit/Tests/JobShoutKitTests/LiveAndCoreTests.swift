import Foundation
@testable import JobShoutAPI
import JobShoutCore
@testable import JobShoutLive
import Testing

@Suite struct LiveEventTests {
    @Test func decodesKnownEventAndTopic() throws {
        let raw = #"{"type":"approval.requested","payload":{"approval_id":"a1","tool_name":"deploy"},"timestamp":"2026-09-29T01:02:03.123456Z","org_id":"o"}"#
        let event = try #require(LiveEvent.decode(Data(raw.utf8)))
        #expect(event.topic == .approvals)
        #expect(event.string("approval_id") == "a1")
        #expect(event.timestamp != nil)
    }

    @Test func toleratesUnknownTypesAndOddPayloads() throws {
        let raw = #"{"type":"something.new","payload":[1,2],"timestamp":"not a date"}"#
        let event = try #require(LiveEvent.decode(Data(raw.utf8)))
        #expect(event.topic == .other)
        #expect(event.payload.isEmpty)
        #expect(event.timestamp == nil)
    }

    @Test func rejectsFramesWithoutType() {
        #expect(LiveEvent.decode(Data(#"{"payload":{}}"#.utf8)) == nil)
    }

    @Test func backoffIsCappedWithJitter() {
        for attempt in 0..<20 {
            let d = LiveEventsClient.backoff(attempt: attempt)
            #expect(d >= .milliseconds(800))
            #expect(d <= .seconds(36))
        }
    }
}

@Suite struct CoreTests {
    @Test func environmentURLs() {
        #expect(AppEnvironment.int.apiBaseURL.absoluteString == "https://int.jobshout.co.uk/api/v1")
        #expect(AppEnvironment.prod.liveEventsURL.absoluteString == "wss://jobshout.co.uk/api/v1/ws")
    }

    @Test func parsesGoTimestamps() throws {
        let t = FlexibleISO8601DateTranscoder()
        _ = try t.decode("2026-09-29T01:02:03.123456Z")
        _ = try t.decode("2026-09-29T01:02:03Z")
        _ = try t.decode("2026-09-29T01:02:03.5+01:00")
        #expect(throws: DecodingError.self) { try t.decode("yesterday") }
    }

    @Test func offlineErrorsReadAsOffline() {
        #expect(APIError.from(URLError(.notConnectedToInternet)).message == "You're offline.")
        #expect(APIError.undocumented(401).isUnauthorized)
    }
}
