package model

// The JobShout.com Content Writer writes long-form articles for the Insights
// hub on jobshout.com. It shares the Article Writer's pipeline and tables — a
// run of either is a blog run — and is told apart by the agent the run is
// attributed to. What differs is what happens after writing: its articles go
// to the CMS as drafts and reach jobshout.com only when someone publishes them
// live, and then they go straight to readers rather than to a review queue.
const (
	BuiltinJobShoutComWriter   = "jobshout_com_writer"
	AgentNameJobShoutComWriter = "JobShout.com Content Writer"
)

// BlogWriters are the builtins a blog run can be attributed to, keyed by the
// value GenerateBlogRequest.Writer takes. Empty is the Article Writer, which is
// what every run written before the second writer existed was.
var BlogWriters = map[string]string{
	BuiltinArticleWriter:     AgentNameArticleWriter,
	BuiltinJobShoutComWriter: AgentNameJobShoutComWriter,
}

// BlogWriterBuiltin is the builtin a Writer value names, defaulting to the
// Article Writer.
func BlogWriterBuiltin(writer string) string {
	if writer == "" {
		return BuiltinArticleWriter
	}
	return writer
}
