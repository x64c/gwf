package throttle

import (
	"encoding/json/v2"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Conf is everything a limiter is built from: its bucket groups, and how the
// in-memory one sweeps what it no longer needs. A group's id is what routes name
// when they throttle, and its numbers are what that group's buckets obey.
//
// CleanupCycleSecs and CleanupOlderThanSecs are read only by the in-memory
// limiter — the one keeping buckets this process must reclaim. The KVDB limiter
// leaves them unused: a bucket there expires on its own once it would be full.
type Conf struct {
	CleanupCycleSecs     int                  `json:"cleanup_cycle_secs"`
	CleanupOlderThanSecs int                  `json:"cleanup_older_than_secs"`
	Groups               map[string]GroupConf `json:"groups"`
}

// GroupConf is one bucket group's rate: a bucket in it holds at most Burst
// tokens, and gains Increment of them every IncrPeriodSecs.
type GroupConf struct {
	Burst          int `json:"burst"`
	Increment      int `json:"increment"`
	IncrPeriodSecs int `json:"incr_period_secs"`
}

// LoadConf reads <appRoot>/config/.throttle.json, rejecting unknown members, and
// validates what it read.
//
//   - Returns: the conf; an error naming the file when it cannot be read, parsed
//     or validated.
//   - Uses: os.ReadFile, json.Unmarshal, throttle.Conf.Validate.
func LoadConf(appRoot string) (Conf, error) {
	path := filepath.Join(appRoot, "config", ".throttle.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return Conf{}, err
	}
	var conf Conf
	if err = json.Unmarshal(b, &conf, json.RejectUnknownMembers(true)); err != nil {
		return Conf{}, fmt.Errorf("throttle conf %s: %w", path, err)
	}
	if err = conf.Validate(); err != nil {
		return Conf{}, fmt.Errorf("throttle conf %s: %w", path, err)
	}
	return conf, nil
}

// Validate refuses a conf no limiter can be built from: a group with a number
// that cannot describe a rate, and a file naming no group at all — an app that
// throttles nothing prepares no limiter.
func (c Conf) Validate() error {
	if len(c.Groups) == 0 {
		return fmt.Errorf("no bucket group")
	}
	for id, g := range c.Groups {
		if id == "" {
			return fmt.Errorf("a bucket group has an empty id")
		}
		if g.Burst < 1 || g.Increment < 1 || g.IncrPeriodSecs < 1 {
			return fmt.Errorf("bucket group %q: burst, increment and incr_period_secs must be 1 or more", id)
		}
	}
	if c.CleanupCycleSecs < 1 || c.CleanupOlderThanSecs < 1 {
		return fmt.Errorf("cleanup_cycle_secs and cleanup_older_than_secs must be 1 or more")
	}
	return nil
}

// BucketConfs is the conf's groups as the limiters take them.
func (c Conf) BucketConfs() map[string]*BucketConf {
	confs := make(map[string]*BucketConf, len(c.Groups))
	for id, g := range c.Groups {
		confs[id] = &BucketConf{
			Burst:      g.Burst,
			Increment:  g.Increment,
			IncrPeriod: time.Duration(g.IncrPeriodSecs) * time.Second,
		}
	}
	return confs
}

// CleanupCycle is how often the in-memory limiter sweeps.
func (c Conf) CleanupCycle() time.Duration {
	return time.Duration(c.CleanupCycleSecs) * time.Second
}

// CleanupOlderThan is how long a bucket must sit untouched before that sweep
// drops it.
func (c Conf) CleanupOlderThan() time.Duration {
	return time.Duration(c.CleanupOlderThanSecs) * time.Second
}
