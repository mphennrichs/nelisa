package app_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/mphennrichs/nelisa/api/internal/contribution/app"
	"github.com/mphennrichs/nelisa/api/internal/contribution/app/fakes"
	"github.com/mphennrichs/nelisa/api/internal/contribution/domain"
)

var indication = app.SubmitInput{
	Submission: domain.Submission{Kind: domain.KindIndication, Link: "https://camara.leg.br/x", Sphere: domain.SphereFederal},
	ClientKey:  "203.0.113.7",
	HumanToken: fakes.HumanToken,
}

func TestSubmitRateLimitPerIP(t *testing.T) {
	w := fakes.New()
	submit := newSubmit(w, fixedClock{now}, rand.Reader, 5, testPendingCap)
	ctx := context.Background()
	for i := range 5 {
		if _, err := submit.Execute(ctx, indication); err != nil {
			t.Fatalf("envio %d: %v", i, err)
		}
	}
	if _, err := submit.Execute(ctx, indication); !errors.Is(err, app.ErrRateLimited) {
		t.Fatalf("sexto envio = %v; want ErrRateLimited", err)
	}
	if len(w.Contributions) != 5 {
		t.Errorf("o sexto envio não pode gravar: %d contribuições", len(w.Contributions))
	}
	other := indication
	other.ClientKey = "203.0.113.8"
	if _, err := submit.Execute(ctx, other); err != nil {
		t.Errorf("outro IP: %v", err)
	}
	// Na hora seguinte o contador recomeça.
	later := newSubmit(w, fixedClock{now.Add(time.Hour)}, rand.Reader, 5, testPendingCap)
	if _, err := later.Execute(ctx, indication); err != nil {
		t.Errorf("hora seguinte: %v", err)
	}
}

func TestInvalidSubmissionsCountTowardsLimit(t *testing.T) {
	w := fakes.New()
	submit := newSubmit(w, fixedClock{now}, rand.Reader, 2, testPendingCap)
	bad := indication
	bad.Link = "https://exemplo.org/x"
	for range 2 {
		if _, err := submit.Execute(context.Background(), bad); !errors.Is(err, domain.ErrNotOfficialSource) {
			t.Fatalf("envio inválido = %v", err)
		}
	}
	if _, err := submit.Execute(context.Background(), indication); !errors.Is(err, app.ErrRateLimited) {
		t.Errorf("envio válido depois de 2 inválidos = %v; want ErrRateLimited", err)
	}
}

func TestRateLimitNeverStoresTheIP(t *testing.T) {
	w := fakes.New()
	submit := newSubmit(w, fixedClock{now}, rand.Reader, 5, testPendingCap)
	if _, err := submit.Execute(context.Background(), indication); err != nil {
		t.Fatal(err)
	}
	if len(w.RateHits) != 1 || len(w.RateKeys) != 1 {
		t.Fatalf("contadores = %d, chaves = %d", len(w.RateHits), len(w.RateKeys))
	}
	for k := range w.RateHits {
		if strings.Contains(k.Subject, indication.ClientKey) || len(k.Subject) != 32 {
			t.Errorf("o contador guarda o IP em vez do HMAC: %q", k.Subject)
		}
		if k.Bucket != domain.BucketSubmit || !k.Window.Equal(now.Truncate(time.Hour)) {
			t.Errorf("contador = %+v", k)
		}
	}
	for _, c := range w.Contributions {
		if strings.Contains(fmt.Sprintf("%+v", c), indication.ClientKey) {
			t.Errorf("a Contribuição guarda o IP")
		}
	}
}

func TestDailyKeyRotates(t *testing.T) {
	w := fakes.New()
	ctx := context.Background()
	l1 := app.NewRateLimiter(fakes.RateCounter{W: w}, fixedClock{now}, rand.Reader)
	l2 := app.NewRateLimiter(fakes.RateCounter{W: w}, fixedClock{now.Add(24 * time.Hour)}, rand.Reader)
	for _, l := range []*app.RateLimiter{l1, l1, l2} {
		if err := l.Allow(ctx, domain.BucketLookup, "203.0.113.7", 10); err != nil {
			t.Fatal(err)
		}
	}
	if len(w.RateKeys) != 2 {
		t.Fatalf("chaves = %d; want uma por dia", len(w.RateKeys))
	}
	var keys [][]byte
	for _, k := range w.RateKeys {
		keys = append(keys, k)
	}
	if bytes.Equal(keys[0], keys[1]) {
		t.Errorf("a chave do dia seguinte repetiu a anterior")
	}
	// O mesmo IP em dias diferentes não dá para ligar: o HMAC muda.
	subjects := map[string]bool{}
	for k := range w.RateHits {
		subjects[k.Subject] = true
	}
	if len(subjects) != 2 {
		t.Errorf("sujeitos = %d; want 2 (um por chave do dia)", len(subjects))
	}
}

func TestPurgeDropsYesterdaysKeyAndOldCounters(t *testing.T) {
	w := fakes.New()
	ctx := context.Background()
	yesterday := now.Add(-24 * time.Hour)
	if err := app.NewRateLimiter(fakes.RateCounter{W: w}, fixedClock{yesterday}, rand.Reader).Allow(ctx, domain.BucketSubmit, "a", 5); err != nil {
		t.Fatal(err)
	}
	if err := app.NewRateLimiter(fakes.RateCounter{W: w}, fixedClock{now}, rand.Reader).Allow(ctx, domain.BucketSubmit, "a", 5); err != nil {
		t.Fatal(err)
	}
	if err := app.NewPurgeRateLimits(fakes.RateCounter{W: w}, fixedClock{now}).Execute(ctx); err != nil {
		t.Fatal(err)
	}
	if _, ok := w.RateKeys[domain.RateDay(yesterday)]; ok || len(w.RateKeys) != 1 {
		t.Errorf("a chave de ontem sobreviveu: %d chaves", len(w.RateKeys))
	}
	if len(w.RateHits) != 1 {
		t.Errorf("contadores = %d; want só o da hora atual", len(w.RateHits))
	}
}

func TestSubmitPausesAtPendingCap(t *testing.T) {
	w := fakes.New()
	submit := newSubmit(w, fixedClock{now}, rand.Reader, testPerHour, 2)
	ctx := context.Background()
	for i := range 2 {
		in := indication
		in.ClientKey = string(rune('a' + i))
		if _, err := submit.Execute(ctx, in); err != nil {
			t.Fatal(err)
		}
	}
	av := app.NewAvailability(fakes.Contributions{W: w}, 2)
	if err := av.Execute(ctx); !errors.Is(err, app.ErrSubmissionsPaused) {
		t.Errorf("availability = %v; want paused", err)
	}
	if _, err := submit.Execute(ctx, indication); !errors.Is(err, app.ErrSubmissionsPaused) {
		t.Fatalf("envio com a Fila no teto = %v", err)
	}
	if len(w.Contributions) != 2 {
		t.Errorf("gravou com a Fila no teto: %d", len(w.Contributions))
	}
	// Pausado não conta no limite do IP.
	if len(w.RateHits) != 2 {
		t.Errorf("contadores = %d; o envio pausado não deve contar", len(w.RateHits))
	}
	// Moderar uma reabre.
	for id, c := range w.Contributions {
		c.State = domain.StateAccepted
		w.Contributions[id] = c
		break
	}
	if err := av.Execute(ctx); err != nil {
		t.Errorf("availability depois de moderar = %v", err)
	}
}

func TestLookupRateLimit(t *testing.T) {
	w := fakes.New()
	lookup := newLookup(w, fixedClock{now}, 3)
	ctx := context.Background()
	for range 3 {
		if _, err := lookup.Execute(ctx, lookupIn("AAAA-AAAA-AAAA")); !errors.Is(err, domain.ErrContributionNotFound) {
			t.Fatalf("consulta = %v", err)
		}
	}
	if _, err := lookup.Execute(ctx, lookupIn("AAAA-AAAA-AAAA")); !errors.Is(err, app.ErrRateLimited) {
		t.Errorf("quarta consulta = %v; want ErrRateLimited", err)
	}
}

func TestSubmitRequiresHumanCheck(t *testing.T) {
	w := fakes.New()
	ctx := context.Background()
	calls := 0
	submit := newSubmitWithHuman(w, fixedClock{now}, rand.Reader, 5, testPendingCap, fakes.Human{Calls: &calls})
	for _, tok := range []string{"", "token-falso"} {
		in := indication
		in.HumanToken = tok
		if _, err := submit.Execute(ctx, in); !errors.Is(err, app.ErrHumanCheckFailed) {
			t.Errorf("token %q = %v; want ErrHumanCheckFailed", tok, err)
		}
	}
	if len(w.Contributions) != 0 {
		t.Fatalf("gravou sem captcha: %d", len(w.Contributions))
	}
	// Sem captcha confirmado, a tentativa conta no limite do IP.
	if len(w.RateHits) != 1 {
		t.Errorf("contadores = %d", len(w.RateHits))
	}
	if calls != 2 {
		t.Errorf("captcha chamado %d vezes", calls)
	}
}

func TestSubmitRefusedWhenHumanCheckIsDown(t *testing.T) {
	w := fakes.New()
	submit := newSubmitWithHuman(w, fixedClock{now}, rand.Reader, 5, testPendingCap, fakes.Human{Down: true})
	if _, err := submit.Execute(context.Background(), indication); !errors.Is(err, app.ErrHumanCheckUnavailable) {
		t.Fatalf("captcha fora do ar = %v", err)
	}
	if len(w.Contributions) != 0 {
		t.Errorf("aceitou sem checagem")
	}
}

func TestRateLimitSparesTheHumanCheck(t *testing.T) {
	w := fakes.New()
	calls := 0
	submit := newSubmitWithHuman(w, fixedClock{now}, rand.Reader, 1, testPendingCap, fakes.Human{Calls: &calls})
	ctx := context.Background()
	if _, err := submit.Execute(ctx, indication); err != nil {
		t.Fatal(err)
	}
	if _, err := submit.Execute(ctx, indication); !errors.Is(err, app.ErrRateLimited) {
		t.Fatalf("segundo envio = %v", err)
	}
	if calls != 1 {
		t.Errorf("o captcha foi chamado %d vezes; acima do limite não deve ser", calls)
	}
}
