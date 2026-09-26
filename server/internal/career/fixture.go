package career

// GoldenJD is a fixture posting used in unit tests (service + pipeline).
const GoldenJD = `# Head of AI Platform

Company: Northwind Labs

We are hiring a Head of AI Platform to lead inference infrastructure.

Requirements:
- 10+ years in distributed systems
- Kubernetes, GPU scheduling, observability
- Staff/principal engineering background

Compensation: £180k–£220k + equity. Remote (UK).

We will not sponsor visas. Must be authorised to work in the UK.

This is a full-time role, not C2C.
`

func GoldenJDForTest() string { return GoldenJD }
