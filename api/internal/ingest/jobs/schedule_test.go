package jobs

import (
	"testing"
	"time"
)

func TestRadarRunsDailyAtSixInBrasilia(t *testing.T) {
	loc, _ := time.LoadLocation("America/Sao_Paulo")
	d := dailyAt{hour: RadarHour, loc: loc}
	cases := map[string]string{
		"2026-11-11T08:59:00Z": "2026-11-11T09:00:00Z", // 05:59 em Brasília
		"2026-11-11T09:00:00Z": "2026-11-12T09:00:00Z", // 06:00 em ponto: a próxima é amanhã
		"2026-11-11T23:00:00Z": "2026-11-12T09:00:00Z",
	}
	for in, want := range cases {
		at, _ := time.Parse(time.RFC3339, in)
		if got := d.Next(at).UTC().Format(time.RFC3339); got != want {
			t.Errorf("Next(%s) = %s, want %s", in, got, want)
		}
	}
}
