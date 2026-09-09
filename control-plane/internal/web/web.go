// Package web embeds the two static pages the control plane serves. They are
// Go html/template sources; the api package parses them once at start.
package web

import _ "embed"

//go:embed login.html
var Login string

//go:embed terminal.html
var Terminal string
