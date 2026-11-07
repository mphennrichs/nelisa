package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type ImportInput struct {
	Chamber    string
	ExternalID string
}

// ImportResult diz qual Proposição corresponde ao registro e se ela foi
// criada agora.
type ImportResult struct {
	PropositionID string
	Created       bool
	// Suggested é quantos Vínculos de outras Casas ficaram sugeridos para
	// o Editor conferir.
	Suggested int
	// SuggestionsUnavailable são as outras Casas que não responderam à
	// procura de candidatos: a Importação seguiu sem sugestões delas, e o
	// Editor pode buscar lá e importar/vincular depois.
	SuggestionsUnavailable []ChamberCode
}

// maxExternalIDLen: ids das Casas são curtos; acima disso é lixo.
const maxExternalIDLen = 64

// Import é a Importação sob demanda (CONTEXT.md): o Editor escolhe um
// registro de uma Casa e ele entra como Proposição em "criado", com
// Tramitação, Vínculo confirmado dessa Casa e Vínculos sugeridos das
// outras. Tudo o que vem das fontes é buscado antes de abrir a transação.
// Falha na Casa escolhida (registro ou Tramitação) aborta sem gravar nada;
// falha ao procurar candidatos nas outras Casas não aborta: a Proposição
// entra sem as sugestões daquela Casa, e o resultado diz qual não
// respondeu. Importar de novo um registro já vinculado devolve a
// Proposição existente.
type Import struct {
	sources  Sources
	catalog  Catalog
	progress ProgressWriter
	tx       TxRunner
	audit    AuditRecorder
}

func NewImport(sources Sources, catalog Catalog, progress ProgressWriter, tx TxRunner, audit AuditRecorder) *Import {
	return &Import{sources: sources, catalog: catalog, progress: progress, tx: tx, audit: audit}
}

func (uc *Import) Execute(ctx context.Context, in ImportInput) (ImportResult, error) {
	actor, err := requireEditor(ctx)
	if err != nil {
		return ImportResult{}, err
	}
	chamber := ChamberCode(in.Chamber)
	src, ok := uc.sources.get(chamber)
	if !ok {
		return ImportResult{}, ErrUnknownChamber
	}
	extID := strings.TrimSpace(in.ExternalID)
	if extID == "" || len(extID) > maxExternalIDLen || strings.ContainsAny(extID, " /?#\t\r\n") {
		return ImportResult{}, ErrInvalidID
	}

	if res, found, err := uc.existing(ctx, chamber, extID); err != nil || found {
		return res, err
	}

	rec, err := src.Fetch(ctx, extID)
	if err != nil {
		return ImportResult{}, fmt.Errorf("buscando %s/%s: %w", chamber, extID, err)
	}
	if err := validRecord(rec, chamber, extID); err != nil {
		return ImportResult{}, err
	}
	rec.Chamber, rec.ExternalID = chamber, extID
	prog, err := src.Progress(ctx, extID)
	if err != nil {
		return ImportResult{}, fmt.Errorf("buscando tramitação de %s/%s: %w", chamber, extID, err)
	}
	candidates, unavailable, err := uc.candidates(ctx, rec)
	if err != nil {
		return ImportResult{}, err
	}

	var out ImportResult
	err = uc.tx.WithinTx(ctx, func(ctx context.Context) error {
		imp, err := uc.catalog.CreateImported(ctx, ImportRequest{
			Record: rec, LatestEventAt: latestAt(prog), EditorID: actor.UserID, Candidates: candidates,
		})
		if err != nil {
			return err
		}
		if _, err := uc.progress.Record(ctx, imp.PropositionID, chamber, prog); err != nil {
			return err
		}
		out = ImportResult{
			PropositionID: imp.PropositionID, Created: true, Suggested: imp.Suggested,
			SuggestionsUnavailable: unavailable,
		}
		meta := map[string]any{
			"chamber": string(chamber), "external_id": extID, "name": imp.Name, "suggested_links": imp.Suggested,
		}
		if len(unavailable) > 0 {
			names := make([]string, 0, len(unavailable))
			for _, c := range unavailable {
				names = append(names, string(c))
			}
			meta["suggestions_unavailable"] = names
		}
		return uc.audit.Record(ctx, AuditEvent{
			ActorID: actor.UserID, Action: ActionImport, TargetType: "proposition", TargetID: imp.PropositionID,
			Metadata: meta,
		})
	})
	if errors.Is(err, ErrLinkTaken) {
		// Outra Importação do mesmo registro ganhou a corrida: a dela vale.
		if res, found, lerr := uc.existing(ctx, chamber, extID); lerr == nil && found {
			return res, nil
		}
	}
	if err != nil {
		return ImportResult{}, err
	}
	return out, nil
}

// existing devolve a Proposição a que o registro já está ligado.
func (uc *Import) existing(ctx context.Context, chamber ChamberCode, extID string) (ImportResult, bool, error) {
	links, err := uc.catalog.LinksByExternal(ctx, chamber, []string{extID})
	if err != nil {
		return ImportResult{}, false, err
	}
	for _, l := range links {
		if l.Confirmed {
			return ImportResult{PropositionID: l.PropositionID}, true, nil
		}
	}
	if len(links) > 0 {
		return ImportResult{}, false, ErrAlreadySuggested
	}
	return ImportResult{}, false, nil
}

// candidates procura a mesma Proposição nas outras Casas pelo nome
// oficial. Só sugere: quem confirma é o Editor (ADR 0006). Uma Casa que
// falha entra em unavailable; só o cancelamento do contexto é erro.
func (uc *Import) candidates(ctx context.Context, rec SourceRecord) (out []Candidate, unavailable []ChamberCode, err error) {
	for _, c := range uc.sources.chambers() {
		if c == rec.Chamber {
			continue
		}
		src, _ := uc.sources.get(c)
		recs, err := src.Search(ctx, SearchQuery{Kind: rec.Kind, Number: rec.Number, Year: rec.Year})
		if errors.Is(err, ErrNotFound) {
			continue // a outra Casa não tem: nada a sugerir
		}
		if err != nil {
			if ctx.Err() != nil {
				return nil, nil, ctx.Err()
			}
			unavailable = append(unavailable, c)
			continue
		}
		if len(recs) > maxHits {
			recs = recs[:maxHits]
		}
		for _, r := range recs {
			if r.ExternalID == "" {
				continue
			}
			out = append(out, Candidate{
				Chamber: c, ExternalID: r.ExternalID,
				SourceName: officialName(r), SourceURL: r.SourceURL,
			})
		}
	}
	return out, unavailable, nil
}

// validRecord confere o mínimo do que a fonte devolveu: é o registro
// pedido e tem nome oficial.
func validRecord(rec SourceRecord, chamber ChamberCode, extID string) error {
	if (rec.ExternalID != "" && rec.ExternalID != extID) || (rec.Chamber != "" && rec.Chamber != chamber) {
		return fmt.Errorf("%w: registro diferente do pedido", errors.Join(ErrInvalidRecord, ErrUnavailable))
	}
	if strings.TrimSpace(rec.Kind) == "" || rec.Number <= 0 || rec.Year <= 0 {
		return fmt.Errorf("%w: sem tipo, número ou ano", errors.Join(ErrInvalidRecord, ErrUnavailable))
	}
	return nil
}

func officialName(r SourceRecord) string {
	return fmt.Sprintf("%s %d/%d", strings.ToUpper(r.Kind), r.Number, r.Year)
}
