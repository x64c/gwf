package sqldbs

import (
	"errors"
	"fmt"
)

// VerifyTablesAgainstSchema checks table declarations — claims — against a
// Schema — facts — and reports every mismatch rather than stopping at the
// first.
//
// A declaration is a subset claim: the table must exist, every declared
// column must exist in it, and the primary key must match exactly — the same
// column set (order aside: the key's equality chain binds by name, so key
// order is the declaration's internal contract, not the schema's) with the
// same auto-increment stance. Columns the declaration does not mention are
// not judged: what a model does not declare it does not touch. Whether an
// undeclared NOT NULL column without a default breaks inserts is the write
// path's business, left to a later, write-aware check.
//
// Meant for boot: pair each group's claims with the CachedSchema of the DB
// the app vets that group against — DBModelGroup.VerifyTablesAgainstSchema
// does exactly that for a group. The claims need not come from a group: a copy job can check
// one model's table against its far-end DB. tables carries the claims, so it
// is a slice — an empty one is the deliberate statement "no claims",
// vacuously holding. An empty result is the claims holding.
func VerifyTablesAgainstSchema(tables []*Table, schema *Schema) []error {
	if schema == nil {
		return []error{fmt.Errorf("VerifyTablesAgainstSchema: nil schema")}
	}
	var errs []error
	for _, tbl := range tables {
		ts, ok := schema.Table(tbl.Name())
		if !ok {
			errs = append(errs, fmt.Errorf("table %q: declared but the database does not report it", tbl.Name()))
			continue
		}
		for _, name := range tbl.Columns().Names() {
			if _, ok := ts.Column(name); !ok {
				errs = append(errs, fmt.Errorf("table %q: declared column %q does not exist", tbl.Name(), name))
			}
		}
		declaredPK := tbl.PKColumns().Names()
		factPK := ts.PKColumns()
		if !equalAsSets(declaredPK, factPK) {
			errs = append(errs, fmt.Errorf("table %q: declared primary key %v but the database reports %v", tbl.Name(), declaredPK, factPK))
		}
		if tbl.PKAutoIncrement() != ts.PKAutoIncrement() {
			errs = append(errs, fmt.Errorf("table %q: declared pkAutoIncrement=%t but the database reports %t", tbl.Name(), tbl.PKAutoIncrement(), ts.PKAutoIncrement()))
		}
	}
	return errs
}

// equalAsSets compares two lists of names ignoring order. Neither side can
// carry duplicates: a Table's key is validated unique, a Schema's reported
// key columns are unique by nature.
func equalAsSets(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	set := make(map[string]bool, len(a))
	for _, s := range a {
		set[s] = true
	}
	for _, s := range b {
		if !set[s] {
			return false
		}
	}
	return true
}

// VerifyAgainstDB checks this one table's claims against a DB's cached schema
// facts — the single-table rung of the vetting ladder, for a cold caller
// holding one table and one DB. Cached means held: a caller wanting fresh
// facts refreshes first.
func (m *Table) VerifyAgainstDB(db DB) error {
	return errors.Join(VerifyTablesAgainstSchema([]*Table{m}, db.CachedSchema())...)
}
