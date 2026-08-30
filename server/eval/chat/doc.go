// Package chat_eval holds the live chatbot intent suite (Plan 5, Suite A).
//
// The suite itself is behind the evallive build tag and never runs in
// `go test ./...`; see suite_a_live_test.go for how to run it. This file
// carries no build tag so the package is always present in the default build —
// otherwise `go test ./eval/chat/` reports a setup failure for a package whose
// only files are all tag-excluded.
package chat_eval
