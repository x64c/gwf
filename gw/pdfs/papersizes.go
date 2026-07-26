package pdfs

type PaperSize struct {
	Name   string
	Width  Length
	Height Length
}

var (
	Letter = PaperSize{Name: "Letter", Width: Length{8.5, Inch}, Height: Length{11, Inch}}
	A4     = PaperSize{Name: "A4", Width: Length{210, MM}, Height: Length{297, MM}}
)
