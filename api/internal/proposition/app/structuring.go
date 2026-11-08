package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mphennrichs/nelisa/api/internal/platform/clock"
	"github.com/mphennrichs/nelisa/api/internal/proposition/domain"
	"github.com/mphennrichs/nelisa/api/internal/proposition/domain/structuring"
)

// OfficialText busca o PDF oficial de uma Versão e devolve o texto de cada
// página. Só busca em Fonte oficial com API (domain.OfficialTextFetchable),
// também a cada redirecionamento. Erros: *TextFetchError.
type OfficialText interface {
	Fetch(ctx context.Context, url string) ([]string, error)
}

// TextFetchError diz por que a busca falhou. Temporary: vale tentar de novo
// (rede, 5xx); senão a Estruturação fica manual na hora.
type TextFetchError struct {
	Failure   domain.StructuringFailure
	Temporary bool
	Err       error
}

func (e *TextFetchError) Error() string {
	return fmt.Sprintf("buscando texto oficial: %s: %v", e.Failure, e.Err)
}

func (e *TextFetchError) Unwrap() error { return e.Err }

// startStructuring cria a Estruturação de uma Versão recém-criada e, se o
// link é de Fonte oficial com API, enfileira a busca na mesma transação.
func startStructuring(ctx context.Context, repo StructuringRepo, sched StructuringScheduler, v domain.Version, now time.Time) error {
	s := domain.NewStructuring(v, now)
	if err := repo.Create(ctx, s); err != nil {
		return err
	}
	if s.State != domain.StructuringFetching {
		return nil
	}
	return sched.ScheduleStructuring(ctx, v.ID)
}

// RunStructuring é o job: busca o PDF oficial, extrai o texto e o quebra em
// Dispositivos. Idempotente: só age se a Estruturação ainda está em
// "fetching" (o Editor pode ter colado o texto nesse meio-tempo).
type RunStructuring struct {
	versions VersionRepo
	repo     StructuringRepo
	text     OfficialText
	tx       TxRunner
	clock    clock.Clock
}

func NewRunStructuring(versions VersionRepo, repo StructuringRepo, text OfficialText, tx TxRunner, clk clock.Clock) *RunStructuring {
	return &RunStructuring{versions: versions, repo: repo, text: text, tx: tx, clock: clk}
}

// RunResult conta o que o job fez, para o log.
type RunResult struct {
	State      domain.StructuringState
	Failure    domain.StructuringFailure
	Provisions int
	Warnings   int
}

// Execute roda uma tentativa. lastAttempt: se a falha for temporária e
// não houver outra tentativa, a Estruturação fica manual (network); senão
// o erro volta para o River tentar de novo.
func (uc *RunStructuring) Execute(ctx context.Context, versionID string, lastAttempt bool) (RunResult, error) {
	s, err := uc.repo.FindByVersion(ctx, versionID)
	if err != nil {
		return RunResult{}, err
	}
	if s.State != domain.StructuringFetching {
		return RunResult{State: s.State, Failure: s.Failure}, nil
	}
	v, err := uc.versions.FindByID(ctx, versionID)
	if err != nil {
		return RunResult{}, err
	}

	// A busca fica fora da transação: pode levar um minuto.
	pages, err := uc.text.Fetch(ctx, v.OfficialTextURL)
	var text string
	var result structuring.Result
	failure := domain.FailureNone
	var fe *TextFetchError
	switch {
	case errors.As(err, &fe) && fe.Temporary && !lastAttempt:
		return RunResult{}, err
	case errors.As(err, &fe):
		failure = fe.Failure
	case err != nil:
		return RunResult{}, err
	default:
		text = structuring.Clean(pages)
		switch {
		case strings.TrimSpace(text) == "":
			failure = domain.FailureNoText // escaneado
		case len([]rune(text)) > domain.MaxSourceTextLen:
			failure = domain.FailureTooLarge
		default:
			result = structuring.Parse(text)
		}
	}

	var out RunResult
	err = uc.tx.WithinTx(ctx, func(ctx context.Context) error {
		cur, err := uc.repo.FindByVersionForUpdate(ctx, versionID)
		if err != nil {
			return err
		}
		if cur.State != domain.StructuringFetching {
			out = RunResult{State: cur.State, Failure: cur.Failure}
			return nil
		}
		prev := cur.Revision
		now := uc.clock.Now()
		if failure != domain.FailureNone {
			cur.Fail(failure, now)
		} else {
			cur.Structured(text, result, "", now)
			if err := uc.repo.ReplaceProvisions(ctx, versionID, domain.ProvisionsFrom(versionID, result)); err != nil {
				return err
			}
		}
		out = RunResult{State: cur.State, Failure: cur.Failure, Provisions: len(result.Provisions), Warnings: len(result.Warnings)}
		return uc.repo.Update(ctx, cur, prev)
	})
	return out, err
}
