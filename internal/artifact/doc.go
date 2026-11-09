// Package artifact holds pure, side-effect-free helpers for Nebi project
// data: pixi.toml parsing, form round-trip and validation, pixi.lock parsing,
// version diffs and archive packaging.
//
// Nothing in this package touches a database, the network, the filesystem or
// a subprocess. Both the client (internal/client/...) and the server
// (internal/server/...) import it, so it must never import either of them.
package artifact
