// Package api is the contract between Momentum's apps and its backend: JSON
// over HTTPS under /v1 (backend plan D2). The server and the client share
// these types, so neither restates the other's, and Client is the apps' side.
//
// Every operation of the apps' storage interfaces has a route here, except
// sync, contexts, and Taskwarrior's configuration checks: backend mode is
// always online (ADR 0002, P1) and has no contexts (D6).
//
// Times are RFC 3339 instants; the apps resolve date expressions before
// sending them (D3). Every mutation carries an IdempotencyKeyHeader, so a
// retry after a timeout cannot apply twice. A failed request answers with an
// Error.
package api
