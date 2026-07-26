package pdfs

import "io"

// Writer — minimal, stream-style, append-only PDF writer. No page navigation
// T: Concrete Template Type -> depends on each implementation
type Writer[T any] interface {
	PaperSize() PaperSize
	Orientation() string
	Unit() LengthUnit

	TemplateStore() *TemplateStore[T]
	ImportPageAsTemplate(filepath string, pageNum int, storeKey string) error

	AddBlankPage()
	AddTemplatePage(storeKey string) bool

	SetFont(family string, style string, size float64)
	SetTextColor(r, g, b int)
	Text(x, y float64, text string)

	SetDrawColor(r, g, b int)
	SetLineWidth(width float64)
	Line(x1, y1, x2, y2 float64)
	Rect(x, y, w, h float64, style string)

	WriteTo(w io.Writer) (int64, error)
	WriteToFile(filepath string) error
	ProduceBytes() ([]byte, error)
}
