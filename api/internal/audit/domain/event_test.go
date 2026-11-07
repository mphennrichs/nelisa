package domain

import (
	"errors"
	"testing"
	"time"
)

func TestNewEventRequiresActionAndTarget(t *testing.T) {
	at := time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC)
	for name, args := range map[string][3]string{
		"no action":      {"", "user", "1"},
		"no target type": {"user.create", " ", "1"},
		"no target id":   {"user.create", "user", ""},
	} {
		if _, err := NewEvent("a", args[0], args[1], args[2], at, nil); !errors.Is(err, ErrInvalidEvent) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	e, err := NewEvent("", "user.create", "user", "1", at, nil)
	if err != nil || e.Metadata == nil || e.ActorID != "" {
		t.Fatalf("system event: %+v, %v", e, err)
	}
}
