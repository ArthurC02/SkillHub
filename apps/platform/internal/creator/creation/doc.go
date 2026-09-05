// Package creation owns durable interactive Skill-creation sessions.
//
// Go and Postgres are the source of truth. Python receives one bounded step
// request and returns a proposal only; it never owns a checkpoint or a queue.
package creation
