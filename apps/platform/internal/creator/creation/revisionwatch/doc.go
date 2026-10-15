// Package revisionwatch tells open creation streams when their session moved:
// one query per tick reads the revision of every watched session, and each
// workspace and the process hold at most a fixed share of streams.
package revisionwatch
