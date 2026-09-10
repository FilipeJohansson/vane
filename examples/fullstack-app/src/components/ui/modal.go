//go:build js && wasm

package ui

import (
	"github.com/filipejohansson/vane/core"
)

// ModalCtrl drives Modal (see Modal.vane) from anywhere in the page that
// holds a reference to it, same controller/view split as the Field pattern
// but for a portal-rendered overlay instead of an inline element.
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
