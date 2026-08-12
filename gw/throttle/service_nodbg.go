//go:build !debug

package throttle

import "time"

func (s *Service) Cleanup(now time.Time) {
	for _, g := range s.groups {
		g.buckets.rangeAll(func(id string, b *Bucket) bool {
			// lock per bucket while checking/removing
			b.mu.Lock()
			last := b.lastCheck
			b.mu.Unlock()
			if now.Sub(last) > s.cleanupOlderThan {
				g.buckets.remove(id)
			}
			return true // continue iteration
		})
	}
}
