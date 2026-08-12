package sqldbs

// ColumnFieldBinding pairs a column with the field that holds its value — one
// statement of "this field IS this column", read by both directions: a scan
// selects destination pointers by column name, and a write takes its column
// names and values from the same pairs. Statement and destinations drawn from
// one list cannot disagree by position.
//
// The pointer is a field's address (*int64, *string, ...), so a binding
// belongs to one model instance: bindings are produced by the instance being
// read into or written from — on the hot path, per operation. That is why the
// fields are bare and nothing here validates: a model writes its bindings as
// a plain slice literal ({"id", &m.ID}, ...), and well-formedness — FieldPtr
// really a pointer, no column bound twice, key covered, names among the
// table's columns — is judged ONCE, cold, by ValidateDBModelBindings at boot.
// A model whose bindings never pass through that gate runs unguarded.
//
// The unkeyed literal is this type's calling convention, and the type is an
// ALIAS to an anonymous struct to make that convention clean under stock
// tooling: vet's composites check flags unkeyed literals of named structs
// imported from another package, and an app of a thousand models would drown
// in findings — but an aliased anonymous struct is owned by no package, so
// the check does not apply, with no flag and no footnote. The alias
// forecloses methods and any future field — acceptable, because the pair IS
// the concept: a third component would be a redesign, not an addition.
//
// Every bound column is writable. Even a database-assigned key or a defaulted
// column accepts an explicit value — and copy jobs depend on writing them
// verbatim. A truly computed column (GENERATED ALWAYS) is the one kind that
// cannot be written; none exists in any schema of ours, and if one arrives it
// gets its own stance rather than a flag on everyone else's bindings.
type ColumnFieldBinding = struct {
	Column   string
	FieldPtr any
}

// columnFieldBindingsProvider provides a model instance's bindings: every
// bound column, the key first, in the model's declared order — the order a
// whole-row read uses. The shape must not vary between instances; boot
// validation judges one zero instance for the type.
type columnFieldBindingsProvider interface {
	ColumnFieldBindings() []ColumnFieldBinding
}
