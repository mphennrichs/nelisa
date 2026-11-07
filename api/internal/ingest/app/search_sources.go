package app

import (
	"context"
	"errors"
	"regexp"
	"strings"
)

type SearchSourcesInput struct {
	Kind   string
	Number int
	Year   int
}

// SearchHit é um registro achado numa Casa e o que o Nelisa já sabe dele.
type SearchHit struct {
	Record SourceRecord
	// LinkedPropositionID é a Proposição a que o registro já está ligado
	// ("" se nenhuma); LinkConfirmed diz se o vínculo é confirmado ou só
	// sugestão.
	LinkedPropositionID string
	LinkConfirmed       bool
}

// ChamberResult é o resultado da busca numa Casa.
type ChamberResult struct {
	Chamber ChamberCode
	Hits    []SearchHit
	// Unavailable: a fonte falhou nesta busca; as outras Casas seguem.
	Unavailable bool
}

var searchKindPattern = regexp.MustCompile(`^[A-Z]{1,10}$`)

// maxHits limita os registros por Casa: o nome oficial bate com 0 ou 1;
// uma fonte que devolve centenas está errada ou hostil.
const maxHits = 20

// SearchSources busca a Proposição pelo nome oficial em todas as Casas
// configuradas, para o Editor escolher o que importar. Só lê: uma Casa fora
// do ar não impede de ver as outras.
type SearchSources struct {
	sources Sources
	catalog Catalog
}

func NewSearchSources(sources Sources, catalog Catalog) *SearchSources {
	return &SearchSources{sources: sources, catalog: catalog}
}

func (uc *SearchSources) Execute(ctx context.Context, in SearchSourcesInput) ([]ChamberResult, error) {
	if _, err := requireEditor(ctx); err != nil {
		return nil, err
	}
	q := SearchQuery{Kind: strings.ToUpper(strings.TrimSpace(in.Kind)), Number: in.Number, Year: in.Year}
	if !searchKindPattern.MatchString(q.Kind) || q.Number <= 0 || q.Number > 1_000_000 || q.Year < 1800 || q.Year > 9999 {
		return nil, ErrInvalidQuery
	}
	out := make([]ChamberResult, 0)
	for _, c := range uc.sources.chambers() {
		src, _ := uc.sources.get(c)
		res := ChamberResult{Chamber: c, Hits: []SearchHit{}}
		recs, err := src.Search(ctx, q)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				out = append(out, res)
				continue
			}
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			res.Unavailable = true
			out = append(out, res)
			continue
		}
		if len(recs) > maxHits {
			recs = recs[:maxHits]
		}
		ids := make([]string, 0, len(recs))
		for _, r := range recs {
			ids = append(ids, r.ExternalID)
		}
		links, err := uc.catalog.LinksByExternal(ctx, c, ids)
		if err != nil {
			return nil, err
		}
		for _, r := range recs {
			r.Chamber = c
			hit := SearchHit{Record: r}
			for _, l := range links {
				if l.ExternalID != r.ExternalID {
					continue
				}
				// Confirmado vence sugestão: é o que diz onde o registro está.
				if hit.LinkedPropositionID == "" || (l.Confirmed && !hit.LinkConfirmed) {
					hit.LinkedPropositionID, hit.LinkConfirmed = l.PropositionID, l.Confirmed
				}
			}
			res.Hits = append(res.Hits, hit)
		}
		out = append(out, res)
	}
	return out, nil
}
