package assets

import "embed"

//go:embed web/* logo.svg loader.gif icon.png
var WebResources embed.FS
