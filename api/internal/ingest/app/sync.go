package app

import (
	"context"
	"errors"
	"fmt"
)

// SyncLink é a Sincronização de um Vínculo de fonte confirmado (sistema,
// rodada pelo worker): Tramitação nova é aplicada direto, sem duplicar o
// que já está gravado, e os dados oficiais da Proposição são atualizados.
// Não mexe no estado de Revisão. Texto novo (Mudança detectada) fica fora
// da v1.
//
// Falha de fonte não grava dado parcial: só anota no Vínculo, para a
// equipe, e devolve o erro (ErrUnavailable → o worker tenta de novo;
// ErrNotFound → não adianta tentar).
type SyncLink struct {
	sources  Sources
	catalog  Catalog
	progress ProgressWriter
	tx       TxRunner
}

func NewSyncLink(sources Sources, catalog Catalog, progress ProgressWriter, tx TxRunner) *SyncLink {
	return &SyncLink{sources: sources, catalog: catalog, progress: progress, tx: tx}
}

// SyncLinkResult resume uma Sincronização.
type SyncLinkResult struct {
	// Skipped: o Vínculo sumiu ou não é mais confirmado; nada a fazer.
	Skipped     bool
	NewProgress int
}

func (uc *SyncLink) Execute(ctx context.Context, linkID string) (SyncLinkResult, error) {
	link, err := uc.catalog.Link(ctx, linkID)
	if errors.Is(err, ErrLinkNotFound) {
		return SyncLinkResult{Skipped: true}, nil
	}
	if err != nil {
		return SyncLinkResult{}, err
	}
	if !link.Confirmed {
		// Sugestão nunca sincroniza: só depois que um Editor confirmar.
		return SyncLinkResult{Skipped: true}, nil
	}
	src, ok := uc.sources.get(link.Chamber)
	if !ok {
		return SyncLinkResult{}, ErrUnknownChamber
	}

	rec, err := src.Fetch(ctx, link.ExternalID)
	if err == nil {
		err = validRecord(rec, link.Chamber, link.ExternalID)
	}
	if err != nil {
		return SyncLinkResult{}, uc.fail(ctx, link, err)
	}
	rec.Chamber, rec.ExternalID = link.Chamber, link.ExternalID
	prog, err := src.Progress(ctx, link.ExternalID)
	if err != nil {
		return SyncLinkResult{}, uc.fail(ctx, link, err)
	}

	var res SyncLinkResult
	err = uc.tx.WithinTx(ctx, func(ctx context.Context) error {
		n, err := uc.progress.Record(ctx, link.PropositionID, link.Chamber, prog)
		if err != nil {
			return err
		}
		res.NewProgress = n
		return uc.catalog.ApplySync(ctx, SyncResult{LinkID: link.ID, Record: rec, LatestEventAt: latestAt(prog)})
	})
	if errors.Is(err, ErrLinkNotFound) {
		return SyncLinkResult{Skipped: true}, nil
	}
	if err != nil {
		return SyncLinkResult{}, uc.fail(ctx, link, err)
	}
	return res, nil
}

// fail anota a falha no Vínculo e devolve o erro original.
func (uc *SyncLink) fail(ctx context.Context, link LinkRef, cause error) error {
	msg := "erro ao gravar a sincronização"
	switch {
	case errors.Is(cause, ErrNotFound):
		msg = "a fonte não encontrou mais este registro"
	case errors.Is(cause, ErrUnavailable):
		msg = "fonte oficial indisponível: " + cause.Error()
	}
	if err := uc.catalog.RecordSyncFailure(ctx, link.ID, msg); err != nil && !errors.Is(err, ErrLinkNotFound) {
		return errors.Join(cause, fmt.Errorf("anotando falha: %w", err))
	}
	return fmt.Errorf("sincronizando %s/%s: %w", link.Chamber, link.ExternalID, cause)
}

// SyncAll enfileira a Sincronização de todos os Vínculos confirmados
// (job periódico do worker). Cada Vínculo vira um job próprio, para uma
// fonte lenta ou fora do ar não atrasar as outras.
type SyncAll struct {
	catalog   Catalog
	scheduler SyncScheduler
}

func NewSyncAll(catalog Catalog, scheduler SyncScheduler) *SyncAll {
	return &SyncAll{catalog: catalog, scheduler: scheduler}
}

func (uc *SyncAll) Execute(ctx context.Context) (int, error) {
	ids, err := uc.catalog.ConfirmedLinkIDs(ctx)
	if err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}
	if err := uc.scheduler.ScheduleSyncs(ctx, ids); err != nil {
		return 0, err
	}
	return len(ids), nil
}
