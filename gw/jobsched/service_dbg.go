//go:build debug

package jobsched

import (
	"log"
	"time"
)

func (s *Service) runOneTimeJobs(now time.Time) {
	key := now.Unix() / 60
	s.mu.Lock()
	jobs := s.oneTimeJobs[key]
	delete(s.oneTimeJobs, key)
	log.Println("[DEBUG] runOneTimeJobs(now) with key: ", key)
	s.mu.Unlock()
	for _, job := range jobs {
		s.runOneTimeJob(job)
	}
}

func (s *Service) runOneTimeJob(job *OneTimeJob) {
	log.Println("[DEBUG] runOneTimeJob() called")
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		err := s.runTask("one-time job", job.ID, job.Task)
		if job.OnFinished != nil {
			s.runCallback("one-time job", job.ID, "OnFinished", func() { job.OnFinished(err) })
		}
		if s.OnOneTimeJobFinished != nil {
			s.runCallback("one-time job", job.ID, "OnOneTimeJobFinished", func() { s.OnOneTimeJobFinished(job, err) })
		}
	}()
}

func (s *Service) runCronJobs(now time.Time) {
	log.Println("[DEBUG] runCronJobs called at", now)
	s.mu.Lock()
	log.Println("[DEBUG] total cron jobs:", len(s.cronJobs))
	// Copy values to a slice so we can unlock early
	jobs := make([]*CronJob, 0, len(s.cronJobs))
	for _, job := range s.cronJobs {
		jobs = append(jobs, job)
	}
	log.Printf("[DEBUG] %d cronjobs copied", len(jobs))
	s.mu.Unlock()
	for _, job := range jobs {
		log.Println("[DEBUG] matching cron job spec for ", job.ID)
		if job.Matches(now) {
			log.Println("[DEBUG] cron job spec MATCHED for ", job.ID)
			s.runCronJob(job)
		}
	}
}

func (s *Service) runCronJob(job *CronJob) {
	log.Println("[DEBUG] runCronJob() called")
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		err := s.runTask("cron job", job.ID, job.Task)
		if job.OnFinished != nil {
			s.runCallback("cron job", job.ID, "OnFinished", func() { job.OnFinished(err) })
		}
		if s.OnCronJobFinished != nil {
			s.runCallback("cron job", job.ID, "OnCronJobFinished", func() { s.OnCronJobFinished(job, err) })
		}
	}()
}
