//go:build js && wasm

package components

import (
	"time"

	"github.com/filipejohansson/vane/core"
)

type toastItem struct {
	id   int
	msg  string
	kind string
}

type ToasterCtrl struct {
	items  *core.Signal[[]toastItem]
	nextID int
}

func NewToasterCtrl() *ToasterCtrl {
	return &ToasterCtrl{
		items: core.NewSignal([]toastItem{}),
	}
}

// Push adds a toast. kind: "success" | "error" | "info". Auto-dismisses after 3s.
func (t *ToasterCtrl) Push(msg, kind string) {
	id := t.nextID
	t.nextID++
	list := t.items.Get()
	t.items.Set(append(list, toastItem{id: id, msg: msg, kind: kind}))
	time.AfterFunc(3*time.Second, func() { t.dismiss(id) })
}

func (t *ToasterCtrl) dismiss(id int) {
	list := t.items.Get()
	next := make([]toastItem, 0, len(list))
	for _, item := range list {
		if item.id != id {
			next = append(next, item)
		}
	}
	t.items.Set(next)
}
