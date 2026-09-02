// Package agentmodules is the one Register call for every JobShout specialist.
//
// All specialists are wired this way: own package, then register here.
// A new agent does not need significant platform changes — register it, do not
// add a switch. See .claude/rules/agent-modules.md.
package agentmodules

import (
	"context"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/agentmodule"
	"github.com/jobshout/server/internal/blog"
	"github.com/jobshout/server/internal/career"
	"github.com/jobshout/server/internal/images"
	"github.com/jobshout/server/internal/mail"
	"github.com/jobshout/server/internal/pentester"
	"github.com/jobshout/server/internal/prreview"
	"github.com/jobshout/server/internal/research"
	"github.com/jobshout/server/internal/service"
)

// Deps is the launch surface each specialist needs. Nil fields mean that
// agent's Launch returns "not configured". Extra chat tools attach via
// agentmodule.SetToolInstaller in the tools package, not from here.
type Deps struct {
	Career   service.CareerService
	Research service.ResearchService
	Blog     service.BlogService
	Mail     service.MailService
	Pentest  service.PentestService
	Reviews  service.ReviewService
	Images   *service.ImageService
}

func init() {
	// Schemas must exist for tests that never call Register with real services.
	// Production upserts Launch via Register from main.
	Register(Deps{})
}

// Register adds every builtin specialist.
//
// All specialists are wired this way: own package, then one Register call.
// A new agent does not need significant platform changes — register it, do not
// add a switch. Extra chat tools attach via SetToolInstaller; NewRegistryWithTools
// iterates the registry rather than naming agents.
func Register(d Deps) {
	// Rail order matches the previous Task Manager BUILTINS list.
	agentmodule.Register(pentester.Module(d.Pentest))
	agentmodule.Register(prreview.Module(d.Reviews))
	agentmodule.Register(mail.Module(d.Mail))
	agentmodule.Register(career.Module(d.Career))
	agentmodule.Register(blog.Module(d.Blog))
	agentmodule.Register(images.Module(imageAdapter{d.Images}))
	agentmodule.Register(research.Module(d.Research))
}

type imageAdapter struct{ svc *service.ImageService }

func (a imageAdapter) Enabled() bool {
	return a.svc != nil && a.svc.Enabled()
}

func (a imageAdapter) Generate(ctx context.Context, orgID, userID uuid.UUID, prompt, source string) (string, *uuid.UUID, error) {
	if a.svc == nil {
		return "", nil, nil
	}
	res, err := a.svc.Generate(ctx, service.GenerateImageRequest{
		OrgID:  orgID,
		UserID: &userID,
		Prompt: prompt,
		Source: source,
	})
	if err != nil {
		return "", nil, err
	}
	return res.URL, res.RecordID, nil
}
