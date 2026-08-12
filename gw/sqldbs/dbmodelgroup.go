package sqldbs

import "fmt"

// dbModelGroupMember is one registered unit: a table claim, a bindings
// validator, or — the common case, a DB model — both. The validator is the
// framework's own ValidateDBModelBindings instantiation, captured by
// registration; model definitions never write one.
type dbModelGroupMember struct {
	table    *Table       // nil for a validation-only member (a projection's second model)
	validate func() error // nil for a table-only claim (no model registered)
}

// DBModelGroup collects a package's DB models so they can be vetted as one
// unit. Each model package holds its own instance — what belongs to a group
// is that package's choice — and the group names no DB: the app states each
// group↔DB pairing where it vets, because a pairing is information only the
// app has. One group may be vetted against several DBs (read here, write
// there), and vetting is stateless: a pairing in, errors out, nothing
// stored.
//
// There is deliberately no shared registry and no name key: a group is a
// list of claims, not a namespace. Two groups may declare the same table
// name for different databases, and one group may hold the same table twice
// — a full model beside a slim projection — and every member is vetted on
// its own.
//
// Registration happens at package initialization and is not synchronized;
// the group is for reading once init is done.
type DBModelGroup struct {
	members []dbModelGroupMember
}

// Every registration is a function taking the group it registers into —
// one calling shape for the whole surface (methods could not take the type
// parameters the model registrations need, and a mixed surface would be
// worse than a uniform one).

// RegisterTableOrPanic declares a table and collects it in one act —
// construction and registration cannot be separated, so no declaration can
// silently miss its group's vetting. For a table that backs a DB model, use
// RegisterDBModelOrPanic, which also collects the model's bindings
// validation. See NewTableOrPanic for why a declaration-level constructor
// may panic.
func RegisterTableOrPanic(g *DBModelGroup, name string, pkColumns []string, pkAutoIncrement bool, nonPKColumns []string) *Table {
	tbl := NewTableOrPanic(name, pkColumns, pkAutoIncrement, nonPKColumns)
	g.members = append(g.members, dbModelGroupMember{table: tbl})
	return tbl
}

// RegisterDBModelOrPanic registers a DB model: its table declared and its
// bindings validation collected in one act — a model registered here cannot
// miss either side of the group's vetting.
//
// The table's non-key columns are DERIVED from the model's bindings — the
// bindings' columns minus the key, in binding order — so the column list is
// written exactly once, in the bindings. Only what bindings cannot carry is
// passed: the table's name, which columns are its key, and the key's
// auto-increment stance. A zero instance supplies the bindings; a bindings
// method is a plain literal, so it is safe to read during package
// initialization.
//
// A key column the bindings do not bind is refused here: such a model could
// not operate at all, and a declaration-level contradiction panics like
// every declaration-level error. Everything subtler waits for boot
// validation.
//
// A second model on the SAME table — a slim projection — registers the same
// way, with the same table name: its claim is simply its own (smaller)
// bindings, verified on its own. A group is a list, not a namespace.
func RegisterDBModelOrPanic[M any, MP DBModel[M]](g *DBModelGroup, name string, pkColumns []string, pkAutoIncrement bool) *Table {
	model := MP(new(M))
	bindings := model.ColumnFieldBindings()
	pkLeft := make(map[string]bool, len(pkColumns))
	for _, pk := range pkColumns {
		pkLeft[pk] = true
	}
	nonPKColumns := make([]string, 0, len(bindings))
	for _, b := range bindings {
		if pkLeft[b.Column] {
			delete(pkLeft, b.Column)
			continue
		}
		nonPKColumns = append(nonPKColumns, b.Column)
	}
	if len(pkLeft) > 0 {
		missing := make([]string, 0, len(pkLeft))
		for pk := range pkLeft {
			missing = append(missing, pk)
		}
		panic(fmt.Sprintf("RegisterDBModelOrPanic: table %q: key column(s) %q not bound by %T", name, missing, model))
	}
	tbl := NewTableOrPanic(name, pkColumns, pkAutoIncrement, nonPKColumns)
	g.members = append(g.members, dbModelGroupMember{table: tbl, validate: ValidateDBModelBindings[M, MP]})
	return tbl
}

// Tables is every table declared so far, in registration order.
func (g *DBModelGroup) Tables() []*Table {
	tables := make([]*Table, 0, len(g.members))
	for _, m := range g.members {
		if m.table != nil {
			tables = append(tables, m.table)
		}
	}
	return tables
}

// VerifyTablesAgainstSchema checks every declared table against schema — the
// claims-vs-facts half of Vet, on its own, carrying the free function's name
// because it is exactly that function over the group's tables.
func (g *DBModelGroup) VerifyTablesAgainstSchema(schema *Schema) []error {
	return VerifyTablesAgainstSchema(g.Tables(), schema)
}

// ValidateBindings runs every collected bindings validator — the
// well-formedness half of Vet, on its own.
func (g *DBModelGroup) ValidateBindings() []error {
	var errs []error
	for _, m := range g.members {
		if m.validate == nil {
			continue
		}
		if err := m.validate(); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

// VetAgainstDB gates the whole group against one DB: every member's table claims
// verified against the DB's cached schema, every member's bindings
// validated. Every failure is returned; a failing group must not serve. An
// app calls this once per group↔DB pairing it means — the same group against
// a second DB is simply a second call. Cached means held: a caller wanting
// fresh facts refreshes first.
func (g *DBModelGroup) VetAgainstDB(db DB) []error {
	return append(g.VerifyTablesAgainstSchema(db.CachedSchema()), g.ValidateBindings()...)
}

// ModelGroupDBPair is one group↔DB pairing for VetDBModelGroupsAgainstDBs — an
// ALIAS to an anonymous struct, like ColumnFieldBinding and for the same
// reason: the pairing list is written as bare {group, db} literals, clean
// under stock vet.
type ModelGroupDBPair = struct {
	ModelGroup *DBModelGroup
	DB         DB
}

// VetDBModelGroupsAgainstDBs vets every pairing — each group against that pair's
// DB — and returns every failure. Stateless sugar over VetAgainstDB, for an
// app gating all its pairings in one statement; the same group may appear
// under several DBs and the same DB under several groups. pairs carries the
// app's statements, so it is a slice.
func VetDBModelGroupsAgainstDBs(pairs []ModelGroupDBPair) []error {
	var errs []error
	for _, p := range pairs {
		errs = append(errs, p.ModelGroup.VetAgainstDB(p.DB)...)
	}
	return errs
}
