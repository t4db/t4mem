// Package memory implements a durable, branchable memory engine for agents.
//
// The current backend is a filesystem-backed MVP that mirrors the event/state/fact
// layout from the design doc. The storage boundary is kept behind the Store
// interface so a t4-backed implementation can replace it later without changing
// the memory API or MCP surface.
package memory
