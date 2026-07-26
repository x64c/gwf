//go:build !debug

package throttle

import "time"

func (s *Service) Cleanup(now time.Time) {
	for _, g := range s.groups {
		g.buckets.Range(func(id, value any) bool {
			b := value.(*Bucket)
			// lock per bucket while checking/removing
			b.mu.Lock()
			last := b.lastCheck
			b.mu.Unlock()
			if now.Sub(last) > s.cleanupOlderThan {
				g.buckets.Delete(id)
			}
			return true // continue iteration
		})
	}
}
