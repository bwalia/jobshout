package model

import (
	"bytes"
	"encoding/json"
)

// OptionalString is a JSON string that distinguishes omitted, null, and value.
// Used on UpdateTaskRequest so assigned_agent_id: null clears the assignment
// instead of being treated as "leave unchanged".
type OptionalString struct {
	Set   bool
	Value *string
}

func (o *OptionalString) UnmarshalJSON(data []byte) error {
	o.Set = true
	if bytes.Equal(data, []byte("null")) {
		o.Value = nil
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	o.Value = &s
	return nil
}

func (o OptionalString) MarshalJSON() ([]byte, error) {
	if !o.Set || o.Value == nil {
		return []byte("null"), nil
	}
	return json.Marshal(*o.Value)
}
