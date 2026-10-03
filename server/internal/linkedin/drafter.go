package linkedin

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/jobshout/server/internal/llm"
	"github.com/jobshout/server/internal/model"
)

// maxArticleChars bounds how much of an article goes into a prompt. A post
// needs the problem, the finding and one concrete detail, all of which an
// article states early; the rest only costs context.
const maxArticleChars = 14000

const maxDraftTokens = 1200

const systemPrompt = `You write LinkedIn posts that share an article the author has published.
You write in the first person as the author, in plain, specific language.
You never invent facts, figures, quotes, customers or results the article does not contain.
You never use hype ("game-changer", "revolutionary", "unlock", "in today's fast-paced world") and never use emojis.
You reply with the post text only: no title, no preamble, no markdown, no quotation marks around it.`

// variantBrief is what makes each variant different.
var variantBrief = map[string]string{
	model.LinkedInVariantTechnical: `Reader: software engineers, platform engineers and technical leads.
Structure:
1. Open with the concrete technical problem in one or two short lines that make an engineer stop scrolling. Name the failure or the cost; do not open with "I wrote" or a question.
2. Say why it happens — the mechanism, in two or three sentences.
3. Give the approach or finding from the article, with one specific detail (a technique, a number, a trade-off or a gotcha) taken from it.
4. One practical takeaway the reader can apply this week.
5. End with one question that invites engineers to share how they handle it.
Technical terms are fine; explain any that are not widely known.`,
	model.LinkedInVariantBusiness: `Reader: founders, managers and non-technical decision makers.
Structure:
1. Open with the business problem in one or two short lines: what it costs in money, time, risk or customers. No jargon; do not open with "I wrote" or a question.
2. Say why it matters now and who feels it.
3. Explain, in plain language, what the article found or recommends. Translate any technical idea into what it changes for the business.
4. The outcome or decision this helps with.
5. End with one question that invites leaders to share their experience.
Avoid acronyms and technical terms unless you explain them in a few words.`,
}

// Drafter writes post text with an LLM.
type Drafter struct {
	LLM llm.Client
	Cfg Config
}

// DraftRequest is one variant for one article.
type DraftRequest struct {
	Article model.LinkedInArticle
	Variant string
	// Notes is the angle or problem the person asked to emphasise.
	Notes string
	// HasLink says whether the post will carry the article as a link card.
	HasLink bool
}

// Draft writes one post.
func (d *Drafter) Draft(ctx context.Context, req DraftRequest) (string, error) {
	if d == nil || d.LLM == nil {
		return "", errors.New("linkedin: no LLM configured for drafting")
	}
	brief, ok := variantBrief[req.Variant]
	if !ok {
		return "", fmt.Errorf("linkedin: unknown variant %q", req.Variant)
	}
	resp, err := d.LLM.Generate(llm.WithStage(ctx, "linkedin_"+req.Variant), llm.GenerateRequest{
		Model:     d.Cfg.Model,
		MaxTokens: maxDraftTokens,
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: systemPrompt},
			{Role: llm.RoleUser, Content: draftPrompt(req, brief)},
		},
	})
	if err != nil {
		return "", err
	}
	text := CleanPost(resp.Content)
	if text == "" {
		return "", errors.New("linkedin: the model returned an empty post")
	}
	return text, nil
}

func draftPrompt(req DraftRequest, brief string) string {
	var b strings.Builder
	b.WriteString("Write a LinkedIn post about the article below.\n\n")
	b.WriteString(brief)
	b.WriteString(`

Rules:
- 120 to 220 words, short paragraphs of one to three sentences, a blank line between paragraphs.
- The first two lines are all most readers see before "see more": make them carry the problem.
- Focus on the problem the article helps solve, not on the article itself.
- Close with at most three relevant hashtags on their own final line, written like #Kubernetes.
`)
	if req.HasLink {
		b.WriteString("- The article is attached as a link card below the post: do not paste a URL; you may say \"link below\" once.\n")
	} else {
		b.WriteString("- Do not include a URL or say \"link below\".\n")
	}
	if n := strings.TrimSpace(req.Notes); n != "" {
		b.WriteString("\nThe author asked you to emphasise: ")
		b.WriteString(n)
		b.WriteString("\n")
	}
	b.WriteString("\nArticle title: ")
	b.WriteString(req.Article.Title)
	b.WriteString("\n\nArticle:\n")
	b.WriteString(truncate(req.Article.Markdown, maxArticleChars))
	return b.String()
}

var (
	thinkBlock = regexp.MustCompile(`(?s)<think>.*?</think>`)
	codeFence  = regexp.MustCompile("(?m)^```[a-zA-Z]*\\s*$")
	boldMarks  = regexp.MustCompile(`\*\*([^*\n]+)\*\*`)
	mdHeading  = regexp.MustCompile(`(?m)^#{1,6}\s+`)
	manyBlanks = regexp.MustCompile(`\n{3,}`)
)

// CleanPost turns a model reply into post text: reasoning blocks, fences and
// markdown emphasis removed, wrapping quotes dropped, and the text kept under
// LinkedIn's limit at a paragraph boundary.
func CleanPost(s string) string {
	s = thinkBlock.ReplaceAllString(s, "")
	s = codeFence.ReplaceAllString(s, "")
	s = boldMarks.ReplaceAllString(s, "$1")
	s = mdHeading.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = manyBlanks.ReplaceAllString(s, "\n\n")
	s = strings.TrimSpace(s)
	if len(s) >= 2 && strings.HasPrefix(s, `"`) && strings.HasSuffix(s, `"`) && !strings.Contains(s[1:len(s)-1], `"`) {
		s = strings.TrimSpace(s[1 : len(s)-1])
	}
	return FitCommentary(s)
}

// FitCommentary keeps text within LinkedIn's limit, cutting at the last
// paragraph (or sentence) break that fits. Escaping adds characters, so the
// limit is checked on the escaped form.
func FitCommentary(s string) string {
	if len([]rune(EscapeCommentary(s))) <= model.LinkedInCommentaryMax {
		return s
	}
	rs := []rune(s)
	for n := len(rs); n > 0; {
		cut := string(rs[:n])
		if i := strings.LastIndex(cut, "\n\n"); i > 0 {
			cut = cut[:i]
		} else if i := strings.LastIndex(cut, ". "); i > 0 {
			cut = cut[:i+1]
		} else {
			cut = string(rs[:n-1])
		}
		cut = strings.TrimSpace(cut)
		if len([]rune(EscapeCommentary(cut))) <= model.LinkedInCommentaryMax {
			return cut
		}
		n = len([]rune(cut))
	}
	return ""
}

func truncate(s string, n int) string {
	rs := []rune(s)
	if len(rs) <= n {
		return s
	}
	return string(rs[:n]) + "\n\n[article continues]"
}
