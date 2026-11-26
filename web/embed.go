// Package web embeds the built frontend so the binary can serve it without
// external files. The dist directory is produced by "pnpm web:build" and must
// exist before any "go build" of the main binary.
package web

import "embed"

// Dist holds the contents of web/dist.
//
//go:embed dist
var Dist embed.FS
