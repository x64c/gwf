//go:build debug

package throttle

import (
	"log"
	"time"
)

func (s *Service) cleanup(now time.Time) {
	log.Printf("[DEBUG][Throttle] cleaning Buckets older than %v", s.cleanupOlderThan)
	cleanCnt := 0
	for gid, g := range s.groups {
		log.Printf("[DEBUG][Throttle] cleaning BucketGroup %q", gid)
		g.buckets.rangeAll(func(id string, b *Bucket) bool {
			// lock per bucket while checking/removing
			b.mu.Lock()
			last := b.lastCheck
			log.Printf("[DEBUG][Throttle] inspecting Bucket id=%q lastCheck=%v", id, last)
			b.mu.Unlock()

			if now.Sub(last) > s.cleanupOlderThan {
				g.buckets.remove(id)
				cleanCnt++
				log.Println("[DEBUG][Throttle] Bucket REMOVED")
			} else {
				log.Println("[DEBUG][Throttle] keeping Bucket")
			}
			return true // continue iteration
		})
	}
	log.Printf("[DEBUG][Throttle] %d Buckets cleaned up", cleanCnt)
}
