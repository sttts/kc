package kc

import _ "embed"

// README stores the project README markdown at build time.
//
//go:embed README.md
var README string
