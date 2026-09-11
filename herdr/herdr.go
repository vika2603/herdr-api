// Package herdr is a Go client for the Herdr socket API and the foundation
// for writing Herdr plugins in Go.
//
// Wire types, result and event decoders, and the typed method wrappers on
// Client are generated from the JSON Schema printed by `herdr api schema
// --json`; the snapshot lives in ../schema. The transport in this package is
// handwritten. See docs/design.md for the layout and the generation rules.
package herdr

//go:generate go run ../cmd/herdr-api-gen -schema ../schema/herdr-api.schema.json -methods ../schema/method-results.json -out .
