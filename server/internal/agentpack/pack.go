package agentpack

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/jobshout/server/internal/agentmodule"
	"github.com/jobshout/server/internal/agentschema"
	"github.com/jobshout/server/internal/model"
)

// Input is everything Pack needs from the origin org. IDs stay in Source only.
type Input struct {
	Agent     *model.Agent
	Tools     []string
	Skills    []Skill
	Knowledge []File
}

// Pack builds a sanitized package from origin data.
func Pack(in Input) (*Package, error) {
	if in.Agent == nil {
		return nil, fmt.Errorf("agent is required")
	}
	a := in.Agent
	builtin := agentschema.BuiltinOf(a)
	pkg := &Package{
		Kind:          Kind,
		SchemaVersion: SchemaVersion,
		ExportedAt:    time.Now().UTC(),
		Source: Source{
			AgentID: a.ID.String(),
			Builtin: builtin,
		},
		Agent: Body{
			Name:          a.Name,
			Role:          a.Role,
			Description:   derefStr(a.Description),
			SystemPrompt:  derefStr(a.SystemPrompt),
			ModelProvider: derefStr(a.ModelProvider),
			ModelName:     derefStr(a.ModelName),
			EngineType:    a.EngineType,
			EngineConfig:  a.EngineConfig,
			Builtin:       builtin,
		},
		Tools:     append([]string(nil), in.Tools...),
		Skills:    append([]Skill(nil), in.Skills...),
		Knowledge: append([]File(nil), in.Knowledge...),
	}
	if builtin != "" {
		pkg.Source.FieldKeys = schemaFieldKeys(builtin)
	}
	SanitizePackage(pkg)
	pkg.Warnings = packWarnings(pkg)
	if err := CheckSize(pkg); err != nil {
		return nil, err
	}
	return pkg, nil
}

func schemaFieldKeys(builtin string) []string {
	s := agentschema.ForBuiltin(builtin)
	keys := make([]string, 0, len(s.Fields))
	for _, f := range s.Fields {
		keys = append(keys, f.Key)
	}
	return keys
}

func packWarnings(pkg *Package) []string {
	out := []string{"Credentials, API keys, and OAuth tokens are not included."}
	if len(pkg.Tools) == 0 {
		out = append(out, "This agent has no tools allowed. It will run without tools unless you add some after import.")
	}
	for _, t := range pkg.Tools {
		if isGated(t) {
			out = append(out, "Gated tool "+t+" is in the package and will be skipped on import unless you opt in.")
		}
	}
	if pkg.Agent.Builtin != "" {
		if m, ok := agentmodule.Lookup(pkg.Agent.Builtin); ok {
			for _, req := range m.Requirements {
				if req.Message != "" {
					out = append(out, req.Message)
				}
			}
		}
	}
	return out
}

func isGated(name string) bool {
	for _, g := range GatedTools {
		if g == name {
			return true
		}
	}
	return false
}

// FilenameSlug is used in Content-Disposition.
func FilenameSlug(name string, exportedAt time.Time) string {
	var b strings.Builder
	lastHyphen := false
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastHyphen = false
			continue
		}
		if !lastHyphen && b.Len() > 0 {
			b.WriteByte('-')
			lastHyphen = true
		}
	}
	s := strings.Trim(b.String(), "-")
	if s == "" {
		s = "agent"
	}
	day := exportedAt.UTC().Format("20060102")
	return s + "-" + day + ".jobshout-agent.json"
}

// CheckSize enforces package limits.
func CheckSize(pkg *Package) error {
	if pkg == nil {
		return fmt.Errorf("package is required")
	}
	if len(pkg.Knowledge) > MaxKnowledgeFiles {
		return fmt.Errorf("too many knowledge files (%d; max %d)", len(pkg.Knowledge), MaxKnowledgeFiles)
	}
	for _, f := range pkg.Knowledge {
		if len(f.Content) > MaxKnowledgeBytes {
			return fmt.Errorf("knowledge file %q exceeds %d bytes", f.Filename, MaxKnowledgeBytes)
		}
	}
	raw, err := json.Marshal(pkg)
	if err != nil {
		return fmt.Errorf("package is not valid JSON")
	}
	if len(raw) > MaxJSONBytes {
		return fmt.Errorf("package exceeds %d bytes", MaxJSONBytes)
	}
	return nil
}
