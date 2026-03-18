package webassets

import "embed"

//go:embed templates/*.html static/*
var FS embed.FS
