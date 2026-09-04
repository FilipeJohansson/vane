//go:build js && wasm

package main

import (
	"github.com/filipejohansson/vane/core"
)

func main() {
	core.Mount("root", App)
	select {}
}
