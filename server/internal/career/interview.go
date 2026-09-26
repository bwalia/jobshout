package career

import (
	"fmt"
	"sort"
	"strings"

	"github.com/jobshout/server/internal/model"
)

// MatchStories ranks the story bank against a JD. Provenance is preserved so
// derived-unverified stories stay labelled.
func MatchStories(stories []model.CareerStory, jd, role string) []model.CareerStory {
	if len(stories) == 0 {
		return []model.CareerStory{}
	}
	hay := strings.ToLower(jd + " " + role)
	type scored struct {
		s model.CareerStory
		n float64
	}
	ranked := make([]scored, 0, len(stories))
	for _, st := range stories {
		blob := strings.ToLower(strings.Join([]string{st.Title, st.Situation, st.Task, st.Action, st.Result, strings.Join(st.Tags, " ")}, " "))
		ranked = append(ranked, scored{s: st, n: keywordOverlap(blob, hay)})
	}
	sort.Slice(ranked, func(i, j int) bool { return ranked[i].n > ranked[j].n })
	out := make([]model.CareerStory, 0, len(ranked))
	for _, r := range ranked {
		out = append(out, r.s)
		if len(out) == 5 {
			break
		}
	}
	return out
}

// InterviewPrep is per-company prep when score ≥ 4.0 or status is already Interview.
func InterviewPrep(app *model.CareerApplication, ev *model.CareerEvaluation, stories []model.CareerStory, profile *model.CareerProfile) model.CareerInterviewPrep {
	out := model.CareerInterviewPrep{
		Stories:        []model.CareerStory{},
		NotLegalAdvice: false,
		NeverSubmit:    true,
	}
	if app != nil {
		out.Company = app.Company
		out.Role = app.Role
		out.Status = app.Status
		if app.Score != nil && *app.Score+1e-9 >= model.RecommendApplyMin {
			out.ScoreFloorMet = true
		}
		if app.Status == model.CareerStatusInterview || app.Status == model.CareerStatusOffer {
			out.ScoreFloorMet = true
		}
	}
	jd := ""
	if ev != nil {
		jd = ev.JDText
		if out.Company == "" {
			out.Company = ev.Company
		}
		if out.Role == "" {
			out.Role = ev.Role
		}
		if ev.Score.Overall+1e-9 >= model.RecommendApplyMin {
			out.ScoreFloorMet = true
		}
	}
	out.Stories = MatchStories(stories, jd, out.Role)

	var b strings.Builder
	fmt.Fprintf(&b, "# Interview prep — %s at %s\n\n", nz(out.Role, "role"), nz(out.Company, "company"))
	if !out.ScoreFloorMet {
		b.WriteString("Score is below 4.0 and status is not Interview. Prep is still drafted; product default is to invest here at ≥ 4.0.\n\n")
	}
	b.WriteString("A human attends. Career Agent drafts.\n\n")
	if ev != nil && strings.TrimSpace(ev.Blocks.F) != "" {
		b.WriteString("## Block F (STAR+R plan)\n\n")
		b.WriteString(strings.TrimSpace(ev.Blocks.F))
		b.WriteString("\n\n")
	}
	if len(out.Stories) == 0 {
		b.WriteString("Story bank is empty. Add STAR+R stories before the interview.\n")
	} else {
		b.WriteString("## Matched stories\n\n")
		for i, st := range out.Stories {
			fmt.Fprintf(&b, "%d. **%s** (%s)\n", i+1, nz(st.Title, "untitled"), nz(st.Provenance, "unspecified"))
			if st.Situation != "" {
				fmt.Fprintf(&b, "   - Situation: %s\n", clip(st.Situation, 280))
			}
		}
	}
	if profile != nil && strings.TrimSpace(profile.Voice) != "" {
		b.WriteString("\nVoice guardrail: stay inside the profile voice-dna. Do not invent metrics.\n")
	}
	out.PrepMarkdown = b.String()
	return out
}

// StoryFromEval proposes a derived-unverified story from Block F when the
// score is high enough. The caller must persist it; provenance stays unverified
// until the user confirms.
func StoryFromEval(ev *model.CareerEvaluation) *model.CareerStory {
	if ev == nil || ev.Score.Overall+1e-9 < model.RecommendApplyMin {
		return nil
	}
	body := strings.TrimSpace(ev.Blocks.F)
	if body == "" {
		return nil
	}
	title := "STAR plan — " + nz(ev.Role, "role") + " @ " + nz(ev.Company, "company")
	return &model.CareerStory{
		Title:      title,
		Situation:  clip(body, 2000),
		Provenance: model.CareerStoryDerived,
		Tags:       []string{ev.Company, ev.Role},
	}
}
