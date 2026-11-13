package model

// Marketplace AI builtins (JobShout.com assistants). Each is its own package
// under server/internal/<name> and registers via agentmodules — no platform
// special cases.

const (
	BuiltinProposalGenerator = "proposal_generator"
	BuiltinProjectBreakdown  = "project_breakdown"
	BuiltinTalentMatchmaker  = "talent_matchmaker"

	AgentNameProposalGenerator = "Proposal Generator"
	AgentNameProjectBreakdown  = "Project Breakdown"
	AgentNameTalentMatchmaker  = "Talent Matchmaker"
)
