package sqldbs

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
)

// ValidateDBModelBindings judges a model TYPE's bindings once, cold, so the
// hot path never has to: it allocates a zero instance and checks that the
// bindings a plain slice literal produced are well-formed. Reflection is fine
// here — this runs at boot, not per operation.
//
// Checks:
//   - at least one binding (a bindingless DBModel can do nothing)
//   - every Column named, none bound twice
//   - every FieldPtr a non-nil pointer — the forgotten-& guard, which moved here
//     when bindings became bare literals
//   - every bound column among the Table's declared columns
//   - every key column bound (Hydrate/Update/Delete locate rows by them)
//
// Binding a SUBSET of the table's columns is legal — a slim projection beside
// a full model — so no completeness is demanded beyond the key.
//
// The bindings' shape must not depend on the instance; one zero instance
// stands for the type.
func ValidateDBModelBindings[M any, MP DBModel[M]]() error {
	model := MP(new(M))
	tbl := model.Table()
	bindings := model.ColumnFieldBindings()

	var problems []string
	if len(bindings) == 0 {
		problems = append(problems, "no bindings")
	}
	seen := make(map[string]bool, len(bindings))
	for i, b := range bindings {
		if b.Column == "" {
			problems = append(problems, fmt.Sprintf("binding %d: empty column name", i))
			continue
		}
		if seen[b.Column] {
			problems = append(problems, fmt.Sprintf("column %q bound twice", b.Column))
		}
		seen[b.Column] = true
		if _, ok := tbl.Columns().Find(b.Column); !ok {
			problems = append(problems, fmt.Sprintf("column %q is not declared on table %q", b.Column, tbl.Name()))
		}
		v := reflect.ValueOf(b.FieldPtr)
		if !v.IsValid() || v.Kind() != reflect.Pointer || v.IsNil() {
			problems = append(problems, fmt.Sprintf("column %q: FieldPtr is not a non-nil pointer (forgotten &?)", b.Column))
		}
	}
	for _, name := range tbl.PKColumns().Names() {
		if !seen[name] {
			problems = append(problems, fmt.Sprintf("key column %q has no binding", name))
		}
	}

	if len(problems) > 0 {
		var zero M
		return fmt.Errorf("bindings of %T (table %q): %s", &zero, tbl.Name(), strings.Join(problems, "; "))
	}
	return nil
}

// VetDBModelAgainstDB gates one DB model against one DB: its table claims verified
// against the DB's cached schema facts, its bindings validated — the
// single-model rung of the vetting ladder, for a cold controller about to
// run a model against a DB (read here, write there: vet the model against
// both, one call each).
func VetDBModelAgainstDB[M any, MP DBModel[M]](db DB) error {
	model := MP(new(M))
	return errors.Join(
		model.Table().VerifyAgainstDB(db),
		ValidateDBModelBindings[M, MP](),
	)
}

// VetDBModelInsert gates one DB model as an INSERTER: everything
// VetDBModelAgainstDB judges, plus the shape an insert needs — every fact
// column the model does NOT bind must be supplied by the database: nullable
// or defaulted. (The key needs no special case: a model's bindings always
// cover its key — validation demands it — so the key is never unbound here.)
// A model failing this can still read and update freely; its first insert
// would be refused by the database, and this judges that refusal cold
// instead of at execute time.
//
// Insert intent is the app's knowledge — a slim projection used read-only is
// legitimate — so this rung is OPT-IN: call it, at boot or before a write
// job, for exactly the models you insert through.
func VetDBModelInsert[M any, MP DBModel[M]](db DB) error {
	if err := VetDBModelAgainstDB[M, MP](db); err != nil {
		return err
	}
	model := MP(new(M))
	tbl := model.Table()
	tblSchema, ok := db.CachedSchema().Table(tbl.Name())
	if !ok {
		return fmt.Errorf("VetDBModelInsert: table %q not in the cached schema", tbl.Name())
	}
	bound := make(map[string]bool, len(tbl.Columns().Names()))
	for _, name := range tbl.Columns().Names() {
		bound[name] = true
	}
	var problems []string
	for _, col := range tblSchema.Columns() {
		if bound[col.Name()] || col.Nullable() || col.HasDefault() {
			continue
		}
		problems = append(problems, fmt.Sprintf("column %q is NOT NULL with no default and %T does not bind it", col.Name(), model))
	}
	if len(problems) > 0 {
		return fmt.Errorf("VetDBModelInsert: table %q: %s", tbl.Name(), strings.Join(problems, "; "))
	}
	return nil
}
