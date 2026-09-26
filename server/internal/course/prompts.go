package course

import (
	"fmt"
	"strings"

	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/research"
)

const systemPrompt = "You are an expert instructional designer and subject-matter author. " +
	"You write accurate, well-structured course material for the stated audience and level. " +
	"You never invent facts, statistics, product features or quotations: use the research notes, " +
	"and when they do not cover something, explain the general principle without specifics."

// languageLine tells the model which language to write in. The tag is
// passed through as-is; models understand BCP 47 tags.
func languageLine(locale string) string {
	if locale == "" || locale == model.CourseDefaultLocale {
		return "Write in English."
	}
	return fmt.Sprintf("Write all learner-facing text in the language with tag %q. Keep code, commands and product names unchanged.", locale)
}

func briefBlock(b model.CourseBrief) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Course topic: %s\n", b.Topic)
	if b.Audience != "" {
		fmt.Fprintf(&sb, "Audience: %s\n", b.Audience)
	}
	fmt.Fprintf(&sb, "Level: %s\n", b.Level)
	if len(b.Focus) > 0 {
		fmt.Fprintf(&sb, "Stay within these focus areas: %s\n", strings.Join(b.Focus, ", "))
	}
	if b.Context != "" {
		fmt.Fprintf(&sb, "Author's notes: %s\n", b.Context)
	}
	return sb.String()
}

// researchNotes lists findings with their source so the writer can ground
// every claim. Capped so a large brief does not crowd out the instructions.
func researchNotes(r *research.Brief, maxFindings int) string {
	if r == nil {
		return ""
	}
	var sb strings.Builder
	if r.Summary != "" {
		fmt.Fprintf(&sb, "Summary: %s\n\n", r.Summary)
	}
	sb.WriteString("Findings:\n")
	for i, f := range r.Findings {
		if i == maxFindings {
			break
		}
		fmt.Fprintf(&sb, "- %s", f.Claim)
		if f.SourceURL != "" {
			fmt.Fprintf(&sb, " (source: %s)", f.SourceURL)
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

func outlinePrompt(b model.CourseBrief, notes string) string {
	return fmt.Sprintf(`Plan a course.

%s
Number of chapters: exactly %d.

Research notes:
%s

%s

Reply with JSON only, in this shape:
{
  "title": "course title, under 90 characters",
  "description": "2-3 sentence course description for the catalogue",
  "level": "%s",
  "category": "one or two word category",
  "tags": ["up to 6 short tags"],
  "learning_outcomes": ["4-6 outcomes, each starting with a verb"],
  "chapters": [
    {"title": "chapter title", "summary": "one sentence", "objectives": ["2-4 objectives"]}
  ]
}
Chapters must build on each other in a sensible teaching order and must not overlap.`,
		briefBlock(b), b.ChapterCount, notes, languageLine(b.Locale), b.Level)
}

func outlineContext(o *model.CourseOutline, current int) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Course: %s\n", o.Title)
	for i, ch := range o.Chapters {
		marker := " "
		if i == current {
			marker = ">"
		}
		fmt.Fprintf(&sb, "%s %d. %s\n", marker, i+1, ch.Title)
	}
	return sb.String()
}

func chapterPrompt(b model.CourseBrief, o *model.CourseOutline, idx int, notes string) string {
	ch := o.Chapters[idx]
	return fmt.Sprintf(`Write chapter %d of a course.

%s
Course outline (current chapter marked >):
%s
Chapter title: %s
Chapter summary: %s
Objectives:
- %s

Research notes:
%s

%s

Write the chapter theory in Markdown:
- Do NOT start with a level-1 heading; the title is shown separately. Use ## and ### headings.
- 900-1500 words. Explain concepts step by step with concrete examples; use fenced code blocks where code helps.
- Do not repeat material that belongs to other chapters.
- End with a "## Key takeaways" section of 3-5 bullets.
- Do NOT include quiz questions, exercises with answers, or HTML tags.
Reply with the Markdown only.`,
		idx+1, briefBlock(b), outlineContext(o, idx), ch.Title, ch.Summary,
		strings.Join(ch.Objectives, "\n- "), notes, languageLine(b.Locale))
}

func reviewPrompt(b model.CourseBrief, title, markdown string) string {
	return fmt.Sprintf(`Review this course chapter for a %s audience (%s level).

Chapter: %s
The chapter is written in the language with tag %q; that is intended, do not flag it.

%s

Check: factual accuracy, clarity for the level, logical order, missing steps, and anything that reads as invented (specific numbers, versions or quotes without support).
Reply with JSON only: {"issues": ["specific, actionable issue", ...]}. Use an empty list when it is ready to publish.`,
		orDefault(b.Audience, "general"), b.Level, title, b.Locale, markdown)
}

func revisePrompt(b model.CourseBrief, title, markdown string, issues []string) string {
	return fmt.Sprintf(`Revise this course chapter to fix every issue listed.

Chapter: %s

Issues:
- %s

Current chapter:
%s

%s
Keep the same structure rules: no level-1 heading, ## and ### headings, a "## Key takeaways" section at the end, no quiz, no HTML tags.
Reply with the full revised Markdown only.`,
		title, strings.Join(issues, "\n- "), markdown, languageLine(b.Locale))
}

func quizPrompt(b model.CourseBrief, title, markdown string) string {
	return fmt.Sprintf(`Write a multiple-choice quiz for this course chapter.

Chapter: %s

%s

%s

Rules:
- 5 questions that test understanding of this chapter, not trivia.
- Each question has exactly 4 distinct options and exactly one correct answer.
- Vary the position of the correct answer.
- Every answer must be supported by the chapter text.
Reply with JSON only:
{"questions": [{"question": "...", "options": ["a","b","c","d"], "correct_index": 0, "explanation": "why the answer is correct"}]}`,
		title, markdown, languageLine(b.Locale))
}

// illustrationPrompt asks the image model for a text-free teaching visual.
// Text in generated images is unreliable and would not be translated.
func illustrationPrompt(courseTitle, chapterTitle, summary string) string {
	return fmt.Sprintf("Clean, modern educational illustration for a course chapter titled %q in the course %q. %s "+
		"Flat vector style, clear composition, soft neutral background. No text, no letters, no logos, no watermarks.",
		chapterTitle, courseTitle, summary)
}

func coverPrompt(o *model.CourseOutline) string {
	return fmt.Sprintf("Course cover artwork for an online course titled %q: %s "+
		"Bold, modern, abstract flat illustration with a clear focal point. No text, no letters, no logos, no watermarks.",
		o.Title, o.Description)
}

func orDefault(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}
