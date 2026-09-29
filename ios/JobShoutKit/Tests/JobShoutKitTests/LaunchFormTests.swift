import JobShoutAPI
@testable import JobShoutFeatures
import Testing

@MainActor @Suite struct LaunchFormTests {
    typealias Field = Components.Schemas.AgentSchemaField

    func schema(_ fields: [Field], requireAny: [Components.Schemas.AgentSchema.RequireAnyPayloadPayload]? = nil) -> AgentSchema {
        AgentSchema(builtin: "x", fields: fields, requireAny: requireAny)
    }

    @Test func appliesDefaultsAndCheckboxFalse() {
        let form = LaunchForm(schema: schema([
            Field(key: "mode", label: "Mode", _type: "select", required: false, _default: "fast"),
            Field(key: "dry_run", label: "Dry run", _type: "checkbox", required: false),
        ]))
        #expect(form.values == ["mode": "fast", "dry_run": "false"])
    }

    @Test func validatesRequiredLengthNumberAndOptions() {
        let form = LaunchForm(schema: schema([
            Field(key: "topic", label: "Topic", _type: "text", required: true, minLength: 5),
            Field(key: "pr", label: "PR", _type: "number", required: true, min: 1),
            Field(key: "tone", label: "Tone", _type: "select", required: false,
                  options: [.init(label: "Calm", value: "calm")]),
        ]))
        #expect(form.submission() == nil)
        #expect(form.fieldErrors().keys.sorted() == ["pr", "topic"])

        form.set("topic", "abc")
        form.set("pr", "0")
        form.set("tone", "loud")
        let errors = form.fieldErrors()
        #expect(errors["topic"]?.contains("5 characters") == true)
        #expect(errors["pr"]?.contains("at least 1") == true)
        #expect(errors["tone"] != nil)

        form.set("topic", "  kubernetes  ")
        form.set("pr", "12")
        form.set("tone", "calm")
        #expect(form.submission() == ["topic": "kubernetes", "pr": "12", "tone": "calm"])
    }

    @Test func requireAnyGroups() {
        let form = LaunchForm(schema: schema(
            [Field(key: "url", label: "URL", required: false), Field(key: "text", label: "Text", required: false)],
            requireAny: [.init(keys: ["url", "text"], question: "Give a URL or paste text.")]))
        #expect(form.groupErrors() == ["Give a URL or paste text."])
        form.set("text", "hello")
        #expect(form.groupErrors().isEmpty)
        #expect(form.submission() == ["text": "hello"])
    }

    @Test func unknownTypesRenderAsText() {
        #expect(Field(key: "r", label: "Repo", _type: "repo", required: false).kind == .text)
        #expect(Field(key: "t", label: "T", required: false).kind == .text)
    }

    @Test func tagsMatchWebEncoding() {
        #expect(Tags.parse(" a@x.com,b@y.com , ,c ") == ["a@x.com", "b@y.com", "c"])
        #expect(Tags.join(["a", "b"]) == "a, b")
    }
}
