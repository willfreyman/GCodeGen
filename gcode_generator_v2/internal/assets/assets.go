// Package assets bundles binary resources (fonts, etc.) used by multiple
// packages in this module so the embed directive doesn't have to be
// duplicated next to every consumer.
package assets

import _ "embed"

//go:embed FreeSansBold.ttf
var FreeSansBoldTTF []byte
