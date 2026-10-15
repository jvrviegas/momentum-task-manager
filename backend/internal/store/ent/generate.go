// Package ent is the store's generated client; run go generate after
// changing ./schema.
package ent

//go:generate go tool ent generate --feature privacy,entql,sql/lock,sql/versioned-migration ./schema
