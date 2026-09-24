// Package audience is the reader a piece of content is written for.
//
// It is the one place that knows a platform engineer and an operations manager
// want different articles out of the same research. Every prompt in the writing
// pipeline — topic discovery, planning, drafting, review, expansion — asks a
// Profile what to say instead of hardcoding "a developer audience", so adding a
// reader is adding a row to this file and nothing else.
//
// A Profile is prompt material, not a schema. The fields are exactly the
// sentences the pipeline needs to vary by reader; everything that is a
// guarantee about output rather than a choice about a reader — markdown only,
// cite with [n], never invent a source — stays in the blog package, because a
// profile must not be able to switch those off.
//
// This package deliberately imports nothing from the rest of the server. model,
// blog and research all consume it, so it cannot depend on any of them.
package audience

import (
	"fmt"
	"strings"
)

// Profile is one reader and the writing decisions that follow from them.
type Profile struct {
	// Key is the stable identifier stored on runs, articles and schedules.
	Key string
	// Label and Hint are what a picker shows.
	Label string
	Hint  string

	// Piece is what this reader is handed — "technical article", "technote",
	// "briefing". It opens the plan, draft and review prompts.
	Piece string
	// Reader completes "…for <Reader>". A person, not a demographic: a model
	// writes differently for "a working software engineer" than for
	// "a technical audience".
	Reader string

	// MinWords and MaxWords bound the draft. MinWords is also the floor the
	// expansion pass checks, because asking for a range is not getting one.
	MinWords int
	MaxWords int
	// Sections is how many headings the plan should produce, phrased as the
	// prompt says it ("Four to six").
	Sections string
	// TitleStyle is the headline constraint for this reader.
	TitleStyle string

	// Voice are extra tone rules for the draft, one bullet each.
	Voice []string
	// Code is the code-block policy. Empty means say nothing about code.
	Code string
	// Diagrams is the diagram policy.
	Diagrams string
	// Jargon says what to do with domain vocabulary.
	Jargon string

	// Expand are the "add substance like this" bullets for the expansion pass.
	// They differ by reader for a real reason: an engineer wants trade-offs and
	// failure modes, a manager wants what it costs and who has already done it.
	Expand []string
	// ReviewChecks are what the critic looks for on top of the shared checks.
	ReviewChecks []string

	// Remit describes what this reader's blog covers, for topic discovery.
	Remit string
	// TopicExample contrasts an event with a topic in this reader's terms.
	// Discovery gets news headlines and has to turn them into subjects.
	TopicExample string
	// Prefer and Reject steer which trending candidates become topics.
	Prefer []string
	Reject []string

	// Tags go on the CMS draft, so two audiences are separable in the CMS
	// without opening the piece.
	Tags []string
}

// DefaultKey is the reader assumed when nothing says otherwise.
//
// It is the developer profile because that is what every article written before
// this package existed was written for. A run, a schedule or an article with no
// audience recorded is not ambiguous — it is a developer piece.
const DefaultKey = "developer"

// profiles is the built-in set, in the order a picker should offer them.
var profiles = []Profile{
	{
		Key:   DefaultKey,
		Label: "Developers — deep dive",
		Hint:  "Full technical article: code, diagrams, trade-offs. The original Article Writer behaviour.",

		Piece:  "technical article",
		Reader: "a developer audience",

		MinWords:   900,
		MaxWords:   1400,
		Sections:   "Four to six",
		TitleStyle: "Make it concrete and under 70 characters. No colons-and-subtitles, no \"A Guide To\".",

		Code:     "Include at least one code block where it genuinely helps.",
		Diagrams: "Include a DIAGRAM where one genuinely helps — see the diagram rules below.",

		Expand: []string{
			"Add the practical detail a working engineer needs: what the trade-offs are,\n  what breaks, what to watch for, what the migration actually involves.",
			"Add or extend a code block where it earns its place.",
		},
		ReviewChecks: []string{
			"Missing or broken code blocks, or code that would not run.",
		},

		Remit:        "software engineering, AI and infrastructure, for a developer audience",
		TopicExample: "\"Cilium 1.18 released\" is an event. \"What a kube-proxy-free datapath changes for\ncluster operators\" is a topic.",
		Prefer: []string{
			"Are genuinely about software, AI or infrastructure. Trending lists carry\n  politics, business and general news — ignore all of it.",
			"Have enough substance for a technical article, not just an announcement.",
			"A working engineer would actually benefit from understanding.",
		},
		Reject: []string{
			"Pure funding, acquisition and company-drama stories.",
			"Anything you cannot imagine a code example or a concrete trade-off in.",
		},

		Tags: []string{"engineering"},
	},
	{
		Key:   "technote",
		Label: "Developers — technote",
		Hint:  "Short, task-shaped how-to. Heavy on commands and code, no throat-clearing.",

		Piece:  "technote",
		Reader: "an engineer who has hit this exact problem and wants it solved",

		MinWords:   450,
		MaxWords:   800,
		Sections:   "Three or four",
		TitleStyle: "Name the task or the problem, not the technology. Under 70 characters, and it should read like something someone typed into a search box.",

		Voice: []string{
			"Get to the work in the first two sentences. No scene-setting, no history\n  of the technology, no \"in today's landscape\".",
			"Write the steps in the order someone performs them, and say what a correct\n  result looks like at each one so the reader can tell if it worked.",
			"Name the versions you are describing. A technote that does not say which\n  version it applies to is worse than none.",
		},
		Code:     "Lead with the code or the commands. Every block must be runnable as written — no placeholders the reader has to guess at, and say what any value they must substitute is.",
		Diagrams: "Include a DIAGRAM only when the order of operations or the moving parts are genuinely hard to hold in your head — see the diagram rules below.",

		Expand: []string{
			"Add the failure modes: what the error actually looks like, and what it means.",
			"Add the verification step that proves it worked, if it is missing.",
			"Extend a code block rather than adding prose around it.",
		},
		ReviewChecks: []string{
			"Preamble. Any sentence before the reader learns what to do is a defect in a\n  technote. Flag the opening if it sets a scene instead of naming the problem.",
			"Steps that cannot be followed: a command with an unexplained placeholder, a\n  step whose result is not stated, or a jump between steps that assumes work\n  the reader was never told to do.",
			"Missing version or platform scope on anything version-dependent.",
			"Missing or broken code blocks, or code that would not run.",
		},

		Remit:        "practical software engineering — the problems people hit while building and operating systems",
		TopicExample: "\"Postgres 18 released\" is an event. \"Moving a busy table to a UUIDv7 primary key\nwithout a long lock\" is a technote.",
		Prefer: []string{
			"Describe a task someone actually has to perform, with a right answer.",
			"Have a failure mode worth warning about — the kind of thing that costs an\n  afternoon when you do not know it.",
			"Can be demonstrated in code, configuration or commands.",
		},
		Reject: []string{
			"Announcements, funding and company news.",
			"Anything where the honest answer is \"it depends\" with nothing to show.",
			"Broad overviews — those belong in a deep dive, not a technote.",
		},

		Tags: []string{"engineering", "how-to"},
	},
	{
		Key:   "business",
		Label: "Business & non-technical",
		Hint:  "Plain English for managers and decision-makers: what it means, what it costs, what to do.",

		Piece:  "briefing",
		Reader: "a business manager who is accountable for the outcome but does not write code",

		MinWords:   700,
		MaxWords:   1100,
		Sections:   "Four or five",
		TitleStyle: "Say what changes for the reader's organisation, in their words rather than the industry's. Under 70 characters, no jargon in the title at all.",

		Voice: []string{
			"Plain English throughout. Write the way you would explain it to a capable\n  colleague from another department — not simplified, not condescending, just\n  free of vocabulary they have no reason to know.",
			"Lead every section with what it means for the reader, then the detail that\n  supports it. Never make them read a mechanism to find out why it matters.",
			"Be concrete about consequences: what it costs, what it saves, what it puts\n  at risk, how long it takes, who has to be involved.",
			"Where the honest answer is \"it depends\", say what it depends on and how the\n  reader would find out — do not leave them with a shrug.",
			"Use an analogy when it genuinely clarifies, once, and then drop it. Do not\n  build the whole piece on a metaphor.",
		},
		Code:     "Do not include code, configuration or command lines. If an implementation detail decides the outcome, say in one sentence what it does and why it matters.",
		Diagrams: "Include a DIAGRAM where it makes a process, a decision or a trade-off clearer — see the diagram rules below. It must be labelled in business terms: no class names, no CLI flags, no YAML.",
		Jargon:   "Every technical term gets a plain-English gloss the first time it appears, in the sentence itself rather than a parenthesis at the end. If a term can be avoided entirely, avoid it.",

		Expand: []string{
			"Add what the reader would have to decide, and what they need to know to\n  decide it.",
			"Add the cost, risk and timeline dimension — what this takes to adopt, and\n  what it looks like when it goes badly.",
			"Add a concrete example of an organisation in this position, where a source\n  supports one.",
		},
		ReviewChecks: []string{
			"UNEXPLAINED JARGON. This is the most important check for this reader. Any\n  technical term, acronym or product name used without a plain-English gloss on\n  first use is a defect — flag every one, and say what the gloss should be.",
			"Mechanism with no consequence attached: a paragraph explaining how something\n  works that never says what it means for the reader's organisation.",
			"Cost, risk, timeline or headcount claims stated as fact with no [n] citation.\n  These are the claims this reader will repeat in a meeting, so they carry the\n  same citation burden a version number does.",
			"Code, configuration or command lines, which do not belong in this piece at all.",
			"Hedging that leaves the reader with nothing: \"organisations should consider\"\n  and \"it depends on your needs\" with no statement of what it depends on.",
			"A diagram labelled in engineering vocabulary rather than the reader's.",
		},

		Remit:        "how technology changes what an organisation can do, what it costs and what it risks — written for the people who decide, not the people who implement",
		TopicExample: "\"OpenAI ships a new model tier\" is an event. \"What cheaper long-context models\nchange about the build-or-buy call on document processing\" is a topic.",
		Prefer: []string{
			"Change a decision somebody has to make: what to buy, what to build, what to\n  stop doing, what to budget for.",
			"Have a consequence that can be described in money, time, risk or capability.",
			"A manager would be embarrassed not to have an opinion on in six months.",
		},
		Reject: []string{
			"Release notes, version bumps and anything whose entire substance is a\n  technical detail.",
			"Pure company drama and funding rounds with no consequence for readers who do\n  not work there.",
			"Anything you can only explain by explaining the implementation first.",
		},

		Tags: []string{"business"},
	},
}

// All returns the built-in profiles in picker order.
func All() []Profile {
	out := make([]Profile, len(profiles))
	copy(out, profiles)
	return out
}

// Option is one entry in a picker. It is defined here rather than reusing a
// model type so this package keeps its zero internal dependencies.
type Option struct {
	Value string
	Label string
	Hint  string
}

// Options is the built-in set as picker entries.
func Options() []Option {
	out := make([]Option, 0, len(profiles))
	for _, p := range profiles {
		out = append(out, Option{Value: p.Key, Label: p.Label, Hint: p.Hint})
	}
	return out
}

// Known reports whether key names a built-in profile. Empty is known: it means
// the default, which is how every run written before audiences existed reads.
func Known(key string) bool {
	k := strings.ToLower(strings.TrimSpace(key))
	if k == "" {
		return true
	}
	for _, p := range profiles {
		if p.Key == k {
			return true
		}
	}
	return false
}

// Normalize trims and lowercases a key, returning "" for the default.
//
// The default is stored as empty rather than as "developer" so an article
// written before this existed and one written with the default chosen are the
// same row, instead of two shapes meaning the same thing.
//
// An unknown key is passed through rather than erased. That is deliberate and
// load-bearing: callers normalize before they validate — the HTTP handler and
// the scheduler both do — so a Normalize that quietly turned "manger" into the
// default would take the typo away from Validate and write a developer article
// for a schedule that asked for managers. Nothing downstream is at risk from
// the unknown value surviving, because For falls back to the default anyway.
func Normalize(key string) string {
	k := strings.ToLower(strings.TrimSpace(key))
	if k == DefaultKey {
		return ""
	}
	return k
}

// For returns the profile for a key, falling back to the default.
//
// It never fails. An unknown key reaching this far means something upstream did
// not validate, and writing a developer article is a better answer to that than
// failing a scheduled run at 2am.
func For(key string) Profile {
	k := strings.ToLower(strings.TrimSpace(key))
	for _, p := range profiles {
		if p.Key == k {
			return p
		}
	}
	return profiles[0]
}

// Label is the display name for a key, for logs and the UI.
func Label(key string) string { return For(key).Label }

// --- prompt fragments -------------------------------------------------------
//
// These render the profile into the sentences the pipeline splices in. They
// live here, not at the call sites, so the two consumers — blog and research —
// cannot drift into describing the same reader differently.

// PlanningLine opens the planning prompt.
func (p Profile) PlanningLine() string {
	return fmt.Sprintf("You are planning %s for %s.", indefinite(p.Piece), p.Reader)
}

// WritingLine opens the drafting prompt.
func (p Profile) WritingLine() string {
	return fmt.Sprintf("You are writing %s for %s.", indefinite(p.Piece), p.Reader)
}

// IndefinitePiece is Piece with its article — "a technote", "an article" — for
// prompts that name the thing mid-sentence.
func (p Profile) IndefinitePiece() string { return indefinite(p.Piece) }

// ReviewLine opens the review prompt.
func (p Profile) ReviewLine() string {
	return fmt.Sprintf("You are reviewing a draft %s before publication. Be a harsh critic.", p.Piece)
}

// ExpandLine opens the expansion prompt.
func (p Profile) ExpandLine(current int) string {
	return fmt.Sprintf("This %s is too short. It is %d words and needs to be %d-%d.",
		p.Piece, current, p.MinWords, p.MaxWords)
}

// DraftRules are the reader-specific requirement bullets for the draft.
//
// The invariants the pipeline guarantees for every reader — pure markdown, one
// H1, cite with [n], no Further Reading — are not here. They belong to the
// pipeline, and a profile must not be able to turn them off.
func (p Profile) DraftRules() string {
	var b strings.Builder
	fmt.Fprintf(&b, "- %d-%d words.\n", p.MinWords, p.MaxWords)
	for _, rule := range []string{p.Code, p.Diagrams, p.Jargon} {
		if rule != "" {
			fmt.Fprintf(&b, "- %s\n", rule)
		}
	}
	for _, v := range p.Voice {
		fmt.Fprintf(&b, "- %s\n", v)
	}
	return strings.TrimRight(b.String(), "\n")
}

// ExpandRules are the "add substance like this" bullets for the expansion pass.
func (p Profile) ExpandRules() string { return bullets(p.Expand) }

// ReviewExtras are this reader's checks, appended to the shared checklist.
func (p Profile) ReviewExtras() string {
	if len(p.ReviewChecks) == 0 {
		return ""
	}
	return bullets(p.ReviewChecks)
}

// DiscoveryLine opens the topic-selection prompt.
func (p Profile) DiscoveryLine() string {
	return fmt.Sprintf("You are choosing what a blog should write about this week. The blog covers\n%s.", p.Remit)
}

// PreferBullets and RejectBullets are the candidate filters for discovery.
func (p Profile) PreferBullets() string { return bullets(p.Prefer) }
func (p Profile) RejectBullets() string { return bullets(p.Reject) }

// --- industry ---------------------------------------------------------------
//
// Industry is free text rather than a profile because the useful values are
// open-ended — "NHS trusts", "commercial property", "3PL logistics" — and
// because it varies independently of the reader: the same sector is briefed
// differently to an engineer and to a manager.

// NormalizeIndustry trims an industry, returning "" for none.
func NormalizeIndustry(industry string) string { return strings.TrimSpace(industry) }

// IndustryBrief frames the piece for one sector. Empty means no framing, which
// is the original behaviour.
func IndustryBrief(industry string) string {
	s := NormalizeIndustry(industry)
	if s == "" {
		return ""
	}
	return fmt.Sprintf(`INDUSTRY — this piece is for readers working in %s. Frame it for them:
choose examples, constraints and consequences from that sector, and say what the
subject means there specifically. Do not pad it with sector vocabulary; if the
subject genuinely applies the same way everywhere, say so rather than pretending
otherwise.`, s)
}

// IndustryDiscoveryBrief steers topic selection toward one sector.
func IndustryDiscoveryBrief(industry string) string {
	s := NormalizeIndustry(industry)
	if s == "" {
		return ""
	}
	return fmt.Sprintf(`SECTOR — these readers work in %s. Prefer subjects that change something
for them. A subject with no bearing on that sector is a worse choice than a
smaller one that does.`, s)
}

// --- helpers ----------------------------------------------------------------

func bullets(items []string) string {
	var b strings.Builder
	for _, s := range items {
		if t := strings.TrimSpace(s); t != "" {
			fmt.Fprintf(&b, "- %s\n", t)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// indefinite picks "a" or "an" for a noun phrase, so the prompts read as
// English rather than as a template with a slot in it.
func indefinite(noun string) string {
	if noun == "" {
		return "a piece"
	}
	switch noun[0] {
	case 'a', 'e', 'i', 'o', 'u', 'A', 'E', 'I', 'O', 'U':
		return "an " + noun
	}
	return "a " + noun
}
