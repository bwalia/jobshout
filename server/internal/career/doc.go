// Package career implements CareerOps behaviour in Go: status machine,
// JD extract/liveness, A–H evaluation, ATS scan, doctor checks, and draft
// artifacts. Sequence is code; prose is the model.
//
// Product rules (never relax): human in the loop; do not recommend applying
// below 4.0/5; Block H only at ≥ 4.5; JD text is untrusted data; no fabricated
// CV claims; Block G never changes the score; no-sponsorship is a hard stop.
//
// Behaviour and block meanings follow CareerOps (santifer/career-ops) v1.31.0,
// MIT licence.
package career
