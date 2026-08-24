package platformtools

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jobshout/server/internal/tools"
)

// fnTool is a compact PlatformTool implementation.
type fnTool struct {
	name        string
	desc        string
	domain      string
	perm        string
	destructive bool
	readOnly    bool
	schema      map[string]any
	run         func(ctx context.Context, input map[string]any) (*Result, error)
}

func newTool(name, desc, domain, perm string, destructive, readOnly bool, schema map[string]any, run func(ctx context.Context, input map[string]any) (*Result, error)) PlatformTool {
	return &fnTool{
		name:        name,
		desc:        desc,
		domain:      domain,
		perm:        perm,
		destructive: destructive,
		readOnly:    readOnly,
		schema:      schema,
		run:         run,
	}
}

func (t *fnTool) Name() string        { return t.name }
func (t *fnTool) Description() string { return t.desc }
func (t *fnTool) Schema() map[string]any {
	if t.schema == nil {
		return tools.ObjectSchema(map[string]any{})
	}
	return t.schema
}
func (t *fnTool) Domain() string     { return t.domain }
func (t *fnTool) Permission() string { return t.perm }
func (t *fnTool) Destructive() bool  { return t.destructive }
func (t *fnTool) ReadOnly() bool     { return t.readOnly }

func (t *fnTool) Run(ctx context.Context, input map[string]any) (*Result, error) {
	return t.run(ctx, input)
}

func (t *fnTool) Execute(ctx context.Context, input map[string]any) (string, error) {
	res, err := t.run(ctx, input)
	if err != nil {
		return "", err
	}
	return marshalJSON(res.Data), nil
}

func (t *fnTool) Parameters() tools.ParameterSchema { return t.Schema() }

// TestingTool builds a PlatformTool for unit tests of the chat loop and guard.
func TestingTool(name, perm string, destructive bool, run func(ctx context.Context, input map[string]any) (*Result, error)) PlatformTool {
	return newTool(name, name, "test", perm, destructive, !destructive, tools.ObjectSchema(map[string]any{}), run)
}

func strArg(input map[string]any, key string) string {
	if input == nil {
		return ""
	}
	v, ok := input[key]
	if !ok || v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	default:
		return strings.TrimSpace(fmt.Sprint(x))
	}
}

func boolArg(input map[string]any, key string, fallback bool) bool {
	if input == nil {
		return fallback
	}
	v, ok := input[key]
	if !ok || v == nil {
		return fallback
	}
	switch x := v.(type) {
	case bool:
		return x
	case string:
		switch strings.ToLower(strings.TrimSpace(x)) {
		case "true", "1", "yes":
			return true
		case "false", "0", "no":
			return false
		}
	}
	return fallback
}

func intArg(input map[string]any, key string, fallback int) int {
	if input == nil {
		return fallback
	}
	v, ok := input[key]
	if !ok || v == nil {
		return fallback
	}
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	default:
		return fallback
	}
}

var (
	errNoIdentity = errors.New("not authenticated")
	errNotInOrg   = errors.New("that resource is not in this organisation")
)
