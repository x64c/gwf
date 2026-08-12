package pgsql

import (
	"fmt"
	"strings"
)

// PoolConf is the REQUIRED pool tuning for every DB pool this client creates.
// The framework ships no pool numbers of its own: every field must be stated
// by the deployment, and a missing field refuses the boot by name. Fields are
// pointers so absence is distinguishable from a stated zero.
//
// The conf is the single source for pool settings — a DSN carrying pool_*
// parameters beside it is two sources for one setting, refused at construction
// rather than resolved by precedence.
type PoolConf struct {
	MaxConns            *int32 `json:"max_conns"`               // >= 1
	MinConns            *int32 `json:"min_conns"`               // >= 0, <= max_conns
	ConnMaxLifetimeSecs *int   `json:"conn_max_lifetime_secs"`  // >= 1
	ConnMaxIdleTimeSecs *int   `json:"conn_max_idle_time_secs"` // >= 1 — pgx has no "off"; unstated, pgx would silently apply its own 30m
}

func (p *PoolConf) validate() error {
	if p == nil {
		return fmt.Errorf(`pool: required conf block missing (state max_conns, min_conns, conn_max_lifetime_secs, conn_max_idle_time_secs)`)
	}
	if p.MaxConns == nil {
		return fmt.Errorf("pool.max_conns: required field missing")
	}
	if *p.MaxConns < 1 {
		return fmt.Errorf("pool.max_conns: %d — must be >= 1 (a huge pool is statable; an unlimited one is not)", *p.MaxConns)
	}
	if p.MinConns == nil {
		return fmt.Errorf("pool.min_conns: required field missing (0 = no warm minimum, stated)")
	}
	if *p.MinConns < 0 {
		return fmt.Errorf("pool.min_conns: %d — must be >= 0", *p.MinConns)
	}
	if *p.MinConns > *p.MaxConns {
		return fmt.Errorf("pool.min_conns (%d) > pool.max_conns (%d) — a minimum above the maximum can never hold", *p.MinConns, *p.MaxConns)
	}
	if p.ConnMaxLifetimeSecs == nil {
		return fmt.Errorf("pool.conn_max_lifetime_secs: required field missing")
	}
	if *p.ConnMaxLifetimeSecs < 1 {
		return fmt.Errorf("pool.conn_max_lifetime_secs: %d — must be >= 1", *p.ConnMaxLifetimeSecs)
	}
	if p.ConnMaxIdleTimeSecs == nil {
		return fmt.Errorf("pool.conn_max_idle_time_secs: required field missing")
	}
	if *p.ConnMaxIdleTimeSecs < 1 {
		return fmt.Errorf("pool.conn_max_idle_time_secs: %d — must be >= 1", *p.ConnMaxIdleTimeSecs)
	}
	return nil
}

// validateDSNCarriesNoPoolParams refuses a DSN that states pool settings —
// the conf's pool block is the single source.
func validateDSNCarriesNoPoolParams(dsn string) error {
	if strings.Contains(dsn, "pool_") {
		return fmt.Errorf("dsn declares pool_* parameters while pool settings come from the conf's pool block — one source only; remove them from the dsn")
	}
	return nil
}
