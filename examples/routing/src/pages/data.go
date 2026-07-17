//go:build js && wasm

package pages

type Person struct {
	ID   string
	Name string
	Role string
	Bio  string
}

var People = []Person{
	{"1", "Alice", "Engineer", "Builds distributed systems and obsesses over latency."},
	{"2", "Bob", "Designer", "Turns ambiguous problems into clear, usable interfaces."},
	{"3", "Charlie", "Product", "Bridges customer needs and engineering constraints."},
	{"4", "Diana", "Engineer", "Specializes in compilers and language tooling."},
	{"5", "Eve", "Designer", "Focuses on accessibility and design systems."},
}
