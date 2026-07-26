package sqldbs

import "context"

// TableMeta holds table-level metadata for a DB model.
// Shared per model type (not per instance) — store as a package-level var and return its pointer from TableMeta().
type TableMeta struct {
	Name          string // table name
	PK            string // primary key column name
	AutoIncrement bool   // whether PK auto-increments
}

// SyncFromDB fetches table metadata from the database schema and updates this TableMeta.
func (m *TableMeta) SyncFromDB(ctx context.Context, db DB) error {
	col, incr, err := db.PKColumnOf(ctx, m.Name)
	if err != nil {
		return err
	}
	m.PK = col
	m.AutoIncrement = incr
	return nil
}
