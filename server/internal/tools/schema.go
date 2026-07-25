package tools

// ParameterSchema is a JSON-Schema object (the same shape you'd pass as a
// function/tool "parameters" or "input_schema") describing a tool's inputs.
type ParameterSchema = map[string]any

// SchemaProvider is an OPTIONAL capability a Tool may implement to advertise a
// machine-readable JSON-Schema for its inputs. It is intentionally separate
// from the core Tool interface: tools that don't implement it still work via
// the ReAct JSON-in-prompt loop. When a provider supports native tool-calling
// AND every tool an agent may use implements SchemaProvider, the executor
// drives the loop with real function definitions instead.
type SchemaProvider interface {
	// Parameters returns a JSON-Schema object describing the tool's inputs.
	Parameters() ParameterSchema
}

// ObjectSchema builds a JSON-Schema object of the common shape
// {type:"object", properties:{...}, required:[...]}. required is always
// emitted as an array (empty when no fields are required) so the serialized
// schema is deterministic across providers.
func ObjectSchema(properties map[string]any, required ...string) ParameterSchema {
	if properties == nil {
		properties = map[string]any{}
	}
	if required == nil {
		required = []string{}
	}
	return map[string]any{
		"type":       "object",
		"properties": properties,
		"required":   required,
	}
}
