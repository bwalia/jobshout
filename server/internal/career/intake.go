package career

import (
	"regexp"
	"strings"

	"github.com/jobshout/server/internal/model"
)

var emailRe = regexp.MustCompile(`(?i)[A-Z0-9._%+\-]+@[A-Z0-9.\-]+\.[A-Z]{2,}`)

// ProposeIntake extracts a confirm-before-write profile patch from a CV/export.
func ProposeIntake(doc string) model.CareerIntakeProposal {
	doc = strings.TrimSpace(doc)
	patch := model.UpdateCareerProfileRequest{}
	cv := doc
	patch.CVMarkdown = &cv
	ident := model.CareerIdentity{}
	if m := emailRe.FindString(doc); m != "" {
		ident.Email = m
	}
	if name := firstHeading(doc); name != "" && !strings.Contains(name, "@") && len(name) < 80 {
		ident.FullName = name
	}
	patch.Identity = &ident
	summary := "Proposed CV and identity from the uploaded document. Confirm before saving — nothing is written yet."
	if ident.FullName != "" {
		summary = "Proposed profile for " + ident.FullName + ". Confirm before saving."
	}
	return model.CareerIntakeProposal{Summary: summary, Patch: patch}
}
