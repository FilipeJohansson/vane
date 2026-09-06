//go:build js && wasm

package store

import "github.com/filipejohansson/vane/core"

// Todo is one item in the list.
type Todo struct {
	ID   int
	Text string
	Done bool
}

// Todos holds every item in the list, in creation order.
var Todos = core.NewSignal([]Todo{})

var nextID = 1

// Add appends a new, not-done todo with the given text.
func Add(text string) {
	Todos.Set(append(Todos.Get(), Todo{ID: nextID, Text: text}))
	nextID++
}

// Toggle flips the Done state of the todo with the given id.
func Toggle(id int) {
	list := Todos.Get()
	next := make([]Todo, len(list))
	copy(next, list)
	for i := range next {
		if next[i].ID == id {
			next[i].Done = !next[i].Done
		}
	}
	Todos.Set(next)
}

// Remove deletes the todo with the given id.
func Remove(id int) {
	list := Todos.Get()
	next := make([]Todo, 0, len(list))
	for _, t := range list {
		if t.ID != id {
			next = append(next, t)
		}
	}
	Todos.Set(next)
}
