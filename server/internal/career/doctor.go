package career

import (
	"fmt"
	"strings"

	"github.com/jobshout/server/internal/model"
)

// Doctor is a deterministic integrity check — no LLM.
func Doctor(profile *model.CareerProfile, apps []model.CareerApplication, pipeline []model.CareerPipelineItem, stories []model.CareerStory) model.CareerDoctorReport {
	rep := model.CareerDoctorReport{OK: true, Warnings: []string{}, Info: []string{}}
	if profile == nil {
		rep.OK = false
		rep.Warnings = append(rep.Warnings, "No career profile for this user.")
		return rep
	}
	if strings.TrimSpace(profile.CVMarkdown) == "" {
		rep.OK = false
		rep.Warnings = append(rep.Warnings, "CV is empty — evaluations will be generic until you paste or intake a CV.")
	}
	if profile.Identity.FullName == "" {
		rep.Warnings = append(rep.Warnings, "Identity name is empty.")
		rep.OK = false
	}
	if len(profile.Targets.Titles) == 0 {
		rep.Info = append(rep.Info, "No target titles — scan will not filter by role.")
	}
	if profile.WorkAuth.NeedsSponsorship {
		rep.Info = append(rep.Info, "Profile needs sponsorship — no-sponsorship postings are a hard stop.")
	}
	open, dead := 0, 0
	for _, p := range pipeline {
		if p.Status == model.CareerPipelineOpen {
			open++
		}
		if p.Liveness == "dead" || p.Liveness == "expired" {
			dead++
		}
	}
	rep.Info = append(rep.Info, fmt.Sprintf("%d pipeline items open, %d marked dead/expired.", open, dead))
	rep.Info = append(rep.Info, fmt.Sprintf("%d tracker rows.", len(apps)))
	if len(stories) == 0 {
		rep.Info = append(rep.Info, "Story bank is empty — Block F will not have STAR+R material yet.")
	}
	unverified := 0
	for _, s := range stories {
		if s.Provenance == model.CareerStoryDerived {
			unverified++
		}
	}
	if unverified > 0 {
		rep.Warnings = append(rep.Warnings, fmt.Sprintf("%d stories are derived-unverified — confirm before using in interviews.", unverified))
	}
	if len(rep.Warnings) > 0 {
		rep.OK = false
	}
	return rep
}
