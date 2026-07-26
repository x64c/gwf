package pdfs

// LengthUnit represents a unit of length for PDF dimensions.
// BaseFactor: 1 of this unit = BaseFactor × (1/360 mm)
type LengthUnit struct {
	Name       string
	BaseFactor int64
}

var (
	MM   = LengthUnit{"mm", 360}
	CM   = LengthUnit{"cm", 3600}
	Inch = LengthUnit{"in", 9144}
	Pt   = LengthUnit{"pt", 127}
)

// Length represents a value with a unit.
type Length struct {
	Value float64
	Unit  LengthUnit
}

// SetUnit converts the value to the target unit in place.
func (l *Length) SetUnit(unit LengthUnit) {
	if l.Unit == unit {
		return
	}
	l.Value = l.Value * float64(l.Unit.BaseFactor) / float64(unit.BaseFactor)
	l.Unit = unit
}
