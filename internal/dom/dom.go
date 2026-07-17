//go:build js && wasm

package dom

import "syscall/js"

var Document = js.Global().Get("document")
var Window = js.Global().Get("window")

// Methods
const (
	SetAttribute           = "setAttribute"
	RemoveAttribute        = "removeAttribute"
	AppendChild            = "appendChild"
	RemoveChild            = "removeChild"
	GetElementById         = "getElementById"
	GetElementsByClassName = "getElementsByClassName"
	QuerySelector          = "querySelector"
	QuerySelectorAll       = "querySelectorAll"
	CreateElement          = "createElement"
	CreateTextNode         = "createTextNode"
	CreateComment          = "createComment"
	CreateDocumentFragment = "createDocumentFragment"
	ReplaceWith            = "replaceWith"
	AddEventListener       = "addEventListener"
	RemoveEventListener    = "removeEventListener"
	Focus                  = "focus"
	PreventDefault         = "preventDefault"
	StopPropagation        = "stopPropagation"
)

// Arguments
const (
	ClassName       = "className"
	TextContent     = "textContent"
	ClassList       = "classList"
	Id              = "id"
	Type            = "type"
	Value           = "value"
	Placeholder     = "placeholder"
	Style           = "style"
	NodeValue       = "nodeValue"
	InnerText       = "innerText"
	FirstChild      = "firstChild"
	Click           = "click"
	ChildNodes      = "childNodes"
	Disabled        = "disabled"
	ActiveElement   = "activeElement"
	DocumentElement = "documentElement"
)

// Event handler properties
const (
	OnClick       = "onclick"
	OnInput       = "oninput"
	OnChange      = "onchange"
	OnSubmit      = "onsubmit"
	OnKeyDown     = "onkeydown"
	OnKeyUp       = "onkeyup"
	OnBlur        = "onblur"
	OnFocus       = "onfocus"
	OnDblClick    = "ondblclick"
	OnMouseEnter  = "onmouseenter"
	OnMouseLeave  = "onmouseleave"
	OnScroll      = "onscroll"
	OnPointerDown = "onpointerdown"
	OnPointerUp   = "onpointerup"
	OnPointerMove = "onpointermove"
	OnTouchStart  = "ontouchstart"
	OnTouchEnd    = "ontouchend"
	OnDragStart   = "ondragstart"
	OnDrop        = "ondrop"
)

// Event names
const (
	EventKeyDown    = "keydown"
	EventMouseMove  = "mousemove"
	EventHashChange = "hashchange"
)
