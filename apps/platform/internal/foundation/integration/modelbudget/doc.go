// Package modelbudget stores the per-call ceiling an operator set for one
// model endpoint, and hands callers the number to send after clamping it
// below the deadline the caller itself will wait for.
package modelbudget
