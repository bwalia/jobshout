package tasklaunch

import "github.com/jobshout/server/internal/agentschema"

// TitleFrom builds the board-task title from the registered schema.
//
// All specialists are wired this way: title rules live on the module.
// A new agent does not need a case here — register it, do not add a switch.
func TitleFrom(kind string, v map[string]string) (title, description string) {
	s := agentschema.ForBuiltin(kind)
	v = s.ApplyDefaults(v)
	return agentschema.TitleFrom(s, v)
}
