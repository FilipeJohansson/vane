//go:build js && wasm

package core

import "syscall/js"

// Style holds common CSS properties as typed Go fields.
// Set only the fields you need, empty strings are not applied.
// Use it in vane syntax: style={core.Style{Color: "red", FontSize: "14px"}}.
//
// For properties not listed here, use a static CSS string:
//
//	style="display:grid;grid-template-areas:'a b'"
type Style struct {
	//* Box model
	Width     string
	Height    string
	MinWidth  string
	MinHeight string
	MaxWidth  string
	MaxHeight string

	Margin       string
	MarginTop    string
	MarginRight  string
	MarginBottom string
	MarginLeft   string

	Padding       string
	PaddingTop    string
	PaddingRight  string
	PaddingBottom string
	PaddingLeft   string

	//* Layout
	Display   string
	Position  string
	Top       string
	Right     string
	Bottom    string
	Left      string
	ZIndex    string
	Overflow  string
	OverflowX string
	OverflowY string

	//* Flexbox
	Flex           string
	FlexDirection  string
	FlexWrap       string
	FlexGrow       string
	FlexShrink     string
	FlexBasis      string
	JustifyContent string
	AlignItems     string
	AlignSelf      string
	AlignContent   string
	Gap            string
	RowGap         string
	ColumnGap      string

	//* Grid
	GridTemplateColumns string
	GridTemplateRows    string
	GridColumn          string
	GridRow             string

	//* Typography
	Color          string
	FontSize       string
	FontWeight     string
	FontFamily     string
	FontStyle      string
	LineHeight     string
	TextAlign      string
	TextDecoration string
	TextTransform  string
	WhiteSpace     string
	WordBreak      string
	LetterSpacing  string

	//* Background
	Background         string
	BackgroundColor    string
	BackgroundImage    string
	BackgroundSize     string
	BackgroundPosition string
	BackgroundRepeat   string

	//* Border
	Border       string
	BorderTop    string
	BorderRight  string
	BorderBottom string
	BorderLeft   string
	BorderRadius string
	BorderColor  string
	BorderWidth  string
	BorderStyle  string
	Outline      string

	//* Effects & interaction
	Opacity        string
	BoxShadow      string
	TextShadow     string
	Transform      string
	Transition     string
	Animation      string
	Cursor         string
	Visibility     string
	PointerEvents  string
	UserSelect     string
	ObjectFit      string
	ObjectPosition string
	ListStyle      string
}

// apply sets all non-empty fields on the JS style object.
func (s Style) apply(style js.Value) {
	set := func(k, v string) {
		if v != "" {
			style.Set(k, v)
		}
	}
	set("width", s.Width)
	set("height", s.Height)
	set("minWidth", s.MinWidth)
	set("minHeight", s.MinHeight)
	set("maxWidth", s.MaxWidth)
	set("maxHeight", s.MaxHeight)
	set("margin", s.Margin)
	set("marginTop", s.MarginTop)
	set("marginRight", s.MarginRight)
	set("marginBottom", s.MarginBottom)
	set("marginLeft", s.MarginLeft)
	set("padding", s.Padding)
	set("paddingTop", s.PaddingTop)
	set("paddingRight", s.PaddingRight)
	set("paddingBottom", s.PaddingBottom)
	set("paddingLeft", s.PaddingLeft)
	set("display", s.Display)
	set("position", s.Position)
	set("top", s.Top)
	set("right", s.Right)
	set("bottom", s.Bottom)
	set("left", s.Left)
	set("zIndex", s.ZIndex)
	set("overflow", s.Overflow)
	set("overflowX", s.OverflowX)
	set("overflowY", s.OverflowY)
	set("flex", s.Flex)
	set("flexDirection", s.FlexDirection)
	set("flexWrap", s.FlexWrap)
	set("flexGrow", s.FlexGrow)
	set("flexShrink", s.FlexShrink)
	set("flexBasis", s.FlexBasis)
	set("justifyContent", s.JustifyContent)
	set("alignItems", s.AlignItems)
	set("alignSelf", s.AlignSelf)
	set("alignContent", s.AlignContent)
	set("gap", s.Gap)
	set("rowGap", s.RowGap)
	set("columnGap", s.ColumnGap)
	set("gridTemplateColumns", s.GridTemplateColumns)
	set("gridTemplateRows", s.GridTemplateRows)
	set("gridColumn", s.GridColumn)
	set("gridRow", s.GridRow)
	set("color", s.Color)
	set("fontSize", s.FontSize)
	set("fontWeight", s.FontWeight)
	set("fontFamily", s.FontFamily)
	set("fontStyle", s.FontStyle)
	set("lineHeight", s.LineHeight)
	set("textAlign", s.TextAlign)
	set("textDecoration", s.TextDecoration)
	set("textTransform", s.TextTransform)
	set("whiteSpace", s.WhiteSpace)
	set("wordBreak", s.WordBreak)
	set("letterSpacing", s.LetterSpacing)
	set("background", s.Background)
	set("backgroundColor", s.BackgroundColor)
	set("backgroundImage", s.BackgroundImage)
	set("backgroundSize", s.BackgroundSize)
	set("backgroundPosition", s.BackgroundPosition)
	set("backgroundRepeat", s.BackgroundRepeat)
	set("border", s.Border)
	set("borderTop", s.BorderTop)
	set("borderRight", s.BorderRight)
	set("borderBottom", s.BorderBottom)
	set("borderLeft", s.BorderLeft)
	set("borderRadius", s.BorderRadius)
	set("borderColor", s.BorderColor)
	set("borderWidth", s.BorderWidth)
	set("borderStyle", s.BorderStyle)
	set("outline", s.Outline)
	set("opacity", s.Opacity)
	set("boxShadow", s.BoxShadow)
	set("textShadow", s.TextShadow)
	set("transform", s.Transform)
	set("transition", s.Transition)
	set("animation", s.Animation)
	set("cursor", s.Cursor)
	set("visibility", s.Visibility)
	set("pointerEvents", s.PointerEvents)
	set("userSelect", s.UserSelect)
	set("objectFit", s.ObjectFit)
	set("objectPosition", s.ObjectPosition)
	set("listStyle", s.ListStyle)
}
