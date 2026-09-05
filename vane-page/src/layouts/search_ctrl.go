//go:build js && wasm

package layouts

import (
	"strings"

	"github.com/filipejohansson/vane/core"
)

// searchCtrl is the controller half of the docs search feature (see
// docs/patterns.md's controller/view split): pure state and logic, no JSX,
// so it lives in a plain .go file, the view half (search_dialog.vane, the
// tag-emitting parts) stays in a .vane file. DocsLayout creates one
// searchCtrl, shared by the trigger button, the global Ctrl/Cmd+K shortcut,
// and the modal itself.
type searchCtrl struct {
	open  *core.Signal[bool]
	query *core.Signal[string]
}

func newSearchCtrl() *searchCtrl {
	return &searchCtrl{open: core.NewSignal(false), query: core.NewSignal("")}
}

func (s *searchCtrl) Open()  { s.query.Set(""); s.open.Set(true) }
func (s *searchCtrl) Close() { s.open.Set(false) }

// setupSearchShortcut opens ctrl on Ctrl+K / Cmd+K from anywhere in the docs
// section.
func setupSearchShortcut(ctrl *searchCtrl) {
	core.OnWindowKeyDown(func(e core.KeyEvent) {
		if strings.EqualFold(e.Key, "k") && (e.Ctrl || e.Meta) {
			e.PreventDefault()
			ctrl.Open()
		}
	})
}
