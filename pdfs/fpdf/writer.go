package fpdf

import (
	"bytes"
	"io"
	"os"
	"strings"

	"github.com/phpdave11/gofpdf"
	"github.com/phpdave11/gofpdf/contrib/gofpdi"
	"github.com/x64c/gwf/gw/pdfs"
	"github.com/x64c/gwf/gw/rw"
)

// Writer - simple PDF writer using gofpdf (Go port of FPDF)
// Supported unit: "pt" only
// Currently Custom Size Not Supported: "Letter" and "A4" Only
// ToDo: Support custom fonts
type Writer struct {
	pdfs.Writer[int] // [Embedded Interface] [To Implement]

	paperSize   pdfs.PaperSize
	orientation string
	unit        pdfs.LengthUnit

	impl      *gofpdf.Fpdf
	templates *pdfs.TemplateStore[int]
}

var knownSizes = map[string]bool{
	"a4": true, "letter": true, "legal": true,
}

// NewWriter creates a new PDF writer. For known paper sizes (Letter, A4, Legal),
// the library's built-in dimensions are used. For custom sizes, dimensions are
// converted from PaperSize to the given unit.
func NewWriter(paperSize pdfs.PaperSize, orientation string, unit pdfs.LengthUnit) *Writer {
	var impl *gofpdf.Fpdf
	if knownSizes[strings.ToLower(paperSize.Name)] {
		impl = gofpdf.New(orientation, unit.Name, paperSize.Name, "")
	} else {
		w := paperSize.Width
		h := paperSize.Height
		w.SetUnit(unit)
		h.SetUnit(unit)
		impl = gofpdf.NewCustom(&gofpdf.InitType{
			OrientationStr: orientation,
			UnitStr:        unit.Name,
			Size:           gofpdf.SizeType{Wd: w.Value, Ht: h.Value},
		})
	}
	return &Writer{
		paperSize:   paperSize,
		orientation: orientation,
		unit:        unit,
		impl:        impl,
		templates:   pdfs.NewTemplateStore[int](),
	}
}

func (w *Writer) PaperSize() pdfs.PaperSize {
	return w.paperSize
}

func (w *Writer) Orientation() string {
	return w.orientation
}

func (w *Writer) Unit() pdfs.LengthUnit {
	return w.unit
}

func (w *Writer) TemplateStore() *pdfs.TemplateStore[int] {
	return w.templates
}

func (w *Writer) ImportPageAsTemplate(filepath string, pageNum int, storeKey string) error {
	// Check filepath exist
	tplID := gofpdi.ImportPage(w.impl, filepath, pageNum, "/MediaBox")
	w.templates.Store(storeKey, tplID)
	return nil
}

func (w *Writer) AddBlankPage() {
	w.impl.AddPage()
}

func (w *Writer) AddTemplatePage(storeKey string) bool {
	template, ok := w.templates.Get(storeKey)
	if !ok {
		return false
	}
	w.impl.AddPage()
	pw := w.paperSize.Width
	ph := w.paperSize.Height
	pw.SetUnit(w.unit)
	ph.SetUnit(w.unit)
	gofpdi.UseImportedTemplate(w.impl, template, 0, 0, pw.Value, ph.Value)
	return true
}

func (w *Writer) SetFont(family string, style string, size float64) {
	w.impl.SetFont(family, style, size)
}

func (w *Writer) SetTextColor(r, g, b int) {
	w.impl.SetTextColor(r, g, b)
}

func (w *Writer) Text(x, y float64, text string) {
	w.impl.Text(x, y, text)
}

func (w *Writer) SetDrawColor(r, g, b int) {
	w.impl.SetDrawColor(r, g, b)
}

func (w *Writer) SetLineWidth(width float64) {
	w.impl.SetLineWidth(width)
}

func (w *Writer) Line(x1, y1, x2, y2 float64) {
	w.impl.Line(x1, y1, x2, y2)
}

func (w *Writer) Rect(x, y, wd, h float64, style string) {
	w.impl.Rect(x, y, wd, h, style)
}

func (w *Writer) WriteTo(writer io.Writer) (int64, error) {
	cw := rw.NewCountWriter(writer)
	err := w.impl.Output(cw)
	return cw.BytesWritten(), err
}

func (w *Writer) WriteToFile(filepath string) error {
	pdfBytes, err := w.ProduceBytes()
	if err != nil {
		return err
	}
	return os.WriteFile(filepath, pdfBytes, 0644)
}

func (w *Writer) ProduceBytes() ([]byte, error) {
	var buf bytes.Buffer
	err := w.impl.Output(&buf)
	return buf.Bytes(), err
}
