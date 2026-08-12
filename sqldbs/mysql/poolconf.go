package mysql

import "fmt"

// PoolConf is the REQUIRED pool tuning for every DB connection this client
// creates. The framework ships no pool numbers of its own: every field must be
// stated by the deployment, and a missing field refuses the boot by name.
// Fields are pointers so absence is distinguishable from a stated zero.
type PoolConf struct {
	MaxOpenConns        *int `json:"max_open_conns"`          // >= 1
	MaxIdleConns        *int `json:"max_idle_conns"`          // >= 0, <= max_open_conns
	ConnMaxLifetimeSecs *int `json:"conn_max_lifetime_secs"`  // >= 1
	ConnMaxIdleTimeSecs *int `json:"conn_max_idle_time_secs"` // >= 0; 0 = no idle expiry (database/sql's own semantics), stated
}

func (p *PoolConf) validate() error {
	if p == nil {
		return fmt.Errorf(`pool: required conf block missing (state max_open_conns, max_idle_conns, conn_max_lifetime_secs, conn_max_idle_time_secs)`)
	}
	if p.MaxOpenConns == nil {
		return fmt.Errorf("pool.max_open_conns: required field missing")
	}
	if *p.MaxOpenConns < 1 {
		return fmt.Errorf("pool.max_open_conns: %d — must be >= 1 (a huge pool is statable; an unlimited one is not)", *p.MaxOpenConns)
	}
	if p.MaxIdleConns == nil {
		return fmt.Errorf("pool.max_idle_conns: required field missing (0 = no idle pool, stated)")
	}
	if *p.MaxIdleConns < 0 {
		return fmt.Errorf("pool.max_idle_conns: %d — must be >= 0", *p.MaxIdleConns)
	}
	if *p.MaxIdleConns > *p.MaxOpenConns {
		return fmt.Errorf("pool.max_idle_conns (%d) > pool.max_open_conns (%d) — database/sql would silently clamp it; state a value that can take effect", *p.MaxIdleConns, *p.MaxOpenConns)
	}
	if p.ConnMaxLifetimeSecs == nil {
		return fmt.Errorf("pool.conn_max_lifetime_secs: required field missing")
	}
	if *p.ConnMaxLifetimeSecs < 1 {
		return fmt.Errorf("pool.conn_max_lifetime_secs: %d — must be >= 1", *p.ConnMaxLifetimeSecs)
	}
	if p.ConnMaxIdleTimeSecs == nil {
		return fmt.Errorf("pool.conn_max_idle_time_secs: required field missing (0 = no idle expiry, stated)")
	}
	if *p.ConnMaxIdleTimeSecs < 0 {
		return fmt.Errorf("pool.conn_max_idle_time_secs: %d — must be >= 0", *p.ConnMaxIdleTimeSecs)
	}
	return nil
}
