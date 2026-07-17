//go:build js && wasm

package components

import (
	"github.com/filipejohansson/vane/core"
)

type ModalCtrl struct {
	open      *core.Signal[bool]
	title     *core.Signal[string]
	body      *core.Signal[string]
	onConfirm func()
}

func NewModalCtrl() *ModalCtrl {
	return &ModalCtrl{
		open:  core.NewSignal(false),
		title: core.NewSignal(""),
		body:  core.NewSignal(""),
	}
}

// Open shows an info modal with a single Close button.
func (m *ModalCtrl) Open(title, body string) {
	m.title.Set(title)
	m.body.Set(body)
	m.onConfirm = nil
	m.open.Set(true)
}

// Confirm shows a modal with Confirm + Cancel; fn is called on confirm.
func (m *ModalCtrl) Confirm(title, body string, fn func()) {
	m.title.Set(title)
	m.body.Set(body)
	m.onConfirm = fn
	m.open.Set(true)
}

func (m *ModalCtrl) Close() {
	m.onConfirm = nil
	m.open.Set(false)
}
