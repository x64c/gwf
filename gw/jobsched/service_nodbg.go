//go:build !debug

package jobsched

import "time"

func (s *Service) runOneTimeJobs(now time.Time) {
	key := now.Unix() / 60
	s.mu.Lock()
	jobs := s.oneTimeJobs[key]
	delete(s.oneTimeJobs, key)
	s.mu.Unlock()
	for _, job := range jobs {
		s.runOneTimeJob(job)
	}
}

func (s *Service) runOneTimeJob(job *OneTimeJob) {
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
	s.mu.Lock()
	// Copy values to a slice so we can unlock early
	jobs := make([]*CronJob, 0, len(s.cronJobs))
	for _, job := range s.cronJobs {
		jobs = append(jobs, job)
	}
	s.mu.Unlock()
	for _, job := range jobs {
		if job.Matches(now) {
			s.runCronJob(job)
		}
	}
}

func (s *Service) runCronJob(job *CronJob) {
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
