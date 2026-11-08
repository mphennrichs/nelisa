package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/mphennrichs/nelisa/api/internal/review/domain"
)

var t0 = time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)

var (
	editor   = domain.Approver{UserID: "ed"}
	reviewer = domain.Approver{UserID: "rev", CanReview: true}
	admin    = domain.Approver{UserID: "adm", CanReview: true, IsAdmin: true}
)

func ready(authors ...string) domain.Candidate {
	return domain.Candidate{HasPending: true, InReview: true, Authors: authors}
}

func codes(a domain.Assessment) []string {
	var out []string
	for _, b := range a.Blockers {
		out = append(out, b.Code)
	}
	return out
}

func TestAssess(t *testing.T) {
	missing := []domain.Blocker{{Code: "summary", Message: "Escreva o Resumo."}}
	cases := []struct {
		name   string
		c      domain.Candidate
		who    domain.Approver
		want   error
		breaks bool
	}{
		{"reviewer approves someone else's work", ready("ana"), reviewer, nil, false},
		{"author cannot approve (four eyes)", ready("ana", "rev"), reviewer, domain.ErrFourEyes, false},
		{"admin author approves without four eyes", ready("adm"), admin, nil, true},
		{"admin not author keeps four eyes", ready("ana"), admin, nil, false},
		{"editor cannot approve", ready("ana"), editor, domain.ErrNotReviewer, false},
		{"no pending version", domain.Candidate{}, reviewer, domain.ErrNoPendingVersion, false},
		{"draft is not approved", domain.Candidate{HasPending: true, Authors: []string{"ana"}}, reviewer, domain.ErrNotInReview, false},
		{"incomplete content", domain.Candidate{HasPending: true, InReview: true, Missing: missing}, reviewer, domain.ErrContentIncomplete, false},
	}
	for _, c := range cases {
		a := domain.Assess(c.c, c.who)
		if err := a.Err(); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v (%v)", c.name, err, c.want, codes(a))
		}
		if a.CanApprove() != (c.want == nil) || a.BreaksFourEyes != c.breaks {
			t.Errorf("%s: can=%v breaks=%v", c.name, a.CanApprove(), a.BreaksFourEyes)
		}
	}
	// O erro de conteúdo diz o que falta.
	var inc *domain.ContentIncompleteError
	err := domain.Assess(domain.Candidate{HasPending: true, InReview: true, Missing: missing}, reviewer).Err()
	if !errors.As(err, &inc) || len(inc.Missing) != 1 || inc.Missing[0].Code != "summary" {
		t.Errorf("incomplete: %v", err)
	}
	// Sem a role, só ela aparece: o resto não muda nada para quem nunca
	// vai poder aprovar.
	if got := codes(domain.Assess(domain.Candidate{HasPending: true, Authors: []string{"ed"}, Missing: missing}, editor)); len(got) != 1 || got[0] != "role" {
		t.Errorf("editor blockers: %v", got)
	}
	// Com a role, todos os impedimentos aparecem para a tela (não só o
	// primeiro).
	authorReviewer := domain.Approver{UserID: "ed", CanReview: true}
	if got := codes(domain.Assess(domain.Candidate{HasPending: true, Authors: []string{"ed"}, Missing: missing}, authorReviewer)); len(got) != 3 {
		t.Errorf("all blockers: %v", got)
	}
}

func TestBadgeAndSecondReview(t *testing.T) {
	withFourEyes := domain.NewApproval("p", "v", domain.Assessment{}, "rev", t0)
	if !withFourEyes.FourEyes || withFourEyes.Badge() != 2 || withFourEyes.PendingSecondReview() {
		t.Errorf("four eyes: %+v", withFourEyes)
	}
	if err := withFourEyes.ConfirmSecondReview(reviewer, nil, t0); !errors.Is(err, domain.ErrNoSecondReviewPending) {
		t.Errorf("second review on four eyes: %v", err)
	}

	a := domain.NewApproval("p", "v", domain.Assessment{BreaksFourEyes: true}, "adm", t0)
	if a.FourEyes || a.Badge() != 1 || !a.PendingSecondReview() {
		t.Fatalf("without four eyes: %+v", a)
	}
	// O próprio aprovador não faz a segunda.
	if err := a.ConfirmSecondReview(admin, []string{"adm"}, t0); !errors.Is(err, domain.ErrFourEyes) {
		t.Errorf("approver: %v", err)
	}
	// Autor também não.
	if err := a.ConfirmSecondReview(reviewer, []string{"adm", "rev"}, t0); !errors.Is(err, domain.ErrFourEyes) {
		t.Errorf("author: %v", err)
	}
	// Editor não revisa.
	if err := a.ConfirmSecondReview(editor, []string{"adm"}, t0); !errors.Is(err, domain.ErrNotReviewer) {
		t.Errorf("editor: %v", err)
	}
	if err := a.ConfirmSecondReview(reviewer, []string{"adm"}, t0.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if a.Badge() != 2 || a.PendingSecondReview() || a.SecondReviewBy != "rev" || !a.SecondReviewAt.Equal(t0.Add(time.Hour)) {
		t.Errorf("after second review: %+v", a)
	}
	other := domain.Approver{UserID: "x", CanReview: true}
	if err := a.ConfirmSecondReview(other, nil, t0); !errors.Is(err, domain.ErrNoSecondReviewPending) {
		t.Errorf("third review: %v", err)
	}
}
