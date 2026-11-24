package jobs_test

import (
	"context"
	"crypto/rand"
	"testing"
	"time"

	"github.com/riverqueue/river"

	"github.com/mphennrichs/nelisa/api/internal/contribution/app"
	"github.com/mphennrichs/nelisa/api/internal/contribution/app/fakes"
	"github.com/mphennrichs/nelisa/api/internal/contribution/domain"
	"github.com/mphennrichs/nelisa/api/internal/contribution/jobs"
	"github.com/mphennrichs/nelisa/api/internal/platform/clock"
)

func TestPurgeWorkerDropsYesterdaysKey(t *testing.T) {
	w := fakes.New()
	ctx := context.Background()
	now := time.Date(2026, 11, 22, 0, 10, 0, 0, time.UTC)
	yesterday := &clock.Fixed{T: now.Add(-20 * time.Minute)}
	if err := app.NewRateLimiter(fakes.RateCounter{W: w}, yesterday, rand.Reader).Allow(ctx, domain.BucketSubmit, "203.0.113.7", 5); err != nil {
		t.Fatal(err)
	}
	worker := jobs.NewPurgeRateLimitsWorker(app.NewPurgeRateLimits(fakes.RateCounter{W: w}, &clock.Fixed{T: now}))
	if err := worker.Work(ctx, &river.Job[jobs.PurgeRateLimitsArgs]{}); err != nil {
		t.Fatal(err)
	}
	if len(w.RateKeys) != 0 || len(w.RateHits) != 0 {
		t.Errorf("sobrou: %d chaves, %d contadores", len(w.RateKeys), len(w.RateHits))
	}
}

func TestPeriodicPurgeRunsOnStart(t *testing.T) {
	ps := jobs.PeriodicJobs()
	if len(ps) != 1 {
		t.Fatalf("jobs periódicos = %d", len(ps))
	}
	if err := jobs.Register(river.NewWorkers(), jobs.NewPurgeRateLimitsWorker(nil)); err != nil {
		t.Errorf("register: %v", err)
	}
}
