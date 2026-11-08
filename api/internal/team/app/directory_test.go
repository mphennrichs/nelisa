package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/mphennrichs/nelisa/api/internal/team/app"
	"github.com/mphennrichs/nelisa/api/internal/team/app/fakes"
	"github.com/mphennrichs/nelisa/api/internal/team/domain"
)

func TestDirectoryLookupNamesAndActive(t *testing.T) {
	w := fakes.NewWorld()
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	ana, _ := domain.NewUser("ana", "Ana", "ana@x.org", "h", domain.RoleEditor, false, now)
	ex, _ := domain.NewUser("ex", "Ex", "ex@x.org", "h", domain.RoleReviewer, false, now)
	ex.Deactivate(now)
	w.Put(ana)
	w.Put(ex)

	got, err := app.NewDirectory(w.Users()).Lookup(context.Background(), []string{"ana", "ex", "sumiu"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got["ana"].Name != "Ana" || !got["ana"].Active || got["ex"].Active {
		t.Errorf("got %+v", got)
	}
	if got, _ := app.NewDirectory(w.Users()).Lookup(context.Background(), nil); len(got) != 0 {
		t.Errorf("empty: %+v", got)
	}
}
