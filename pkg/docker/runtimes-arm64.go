//go:build arm64
// +build arm64

package docker

import "embed"

//go:embed python-arm64
var python embed.FS

const runtimesDir = "python-arm64"
