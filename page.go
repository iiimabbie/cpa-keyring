package main

import (
	_ "embed"
	"strings"
)

const version = "0.1.0"

//go:embed page.html
var pageHTML string

var page = strings.ReplaceAll(pageHTML, "__KEYRING_VERSION__", version)
