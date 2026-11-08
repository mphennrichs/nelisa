package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mphennrichs/nelisa/api/internal/platform/clock"
	"github.com/mphennrichs/nelisa/api/internal/proposition/domain"
)

// Catalog são as operações de sistema sobre Proposições e Vínculos que a
// Importação e a Sincronização (módulo ingest, ligado no main) usam. Não
// autorizam nem auditam e não abrem transação: quem orquestra (o caso de
// uso do ingest) já autorizou, audita e chama tudo dentro da própria
// transação. Nunca confirmam um Vínculo sugerido: só ConfirmLink faz isso.
type Catalog struct {
	props        PropositionRepo
	versions     VersionRepo
	contents     ContentRepo
	structurings StructuringRepo
	links        LinkRepo
	sched        StructuringScheduler
	clock        clock.Clock
}

func NewCatalog(repos EditorialRepos, links LinkRepo, sched StructuringScheduler, clk clock.Clock) *Catalog {
	return &Catalog{
		props: repos.Props, versions: repos.Versions, contents: repos.Contents, structurings: repos.Structurings,
		links: links, sched: sched, clock: clk,
	}
}

// LinksByExternal devolve os Vínculos (sugeridos e confirmados) desses
// registros de uma Casa.
func (c *Catalog) LinksByExternal(ctx context.Context, chamber domain.Chamber, externalIDs []string) ([]domain.SourceLink, error) {
	if len(externalIDs) == 0 {
		return nil, nil
	}
	return c.links.ListByExternal(ctx, chamber, externalIDs)
}

// Link devolve um Vínculo pelo id.
func (c *Catalog) Link(ctx context.Context, id string) (domain.SourceLink, error) {
	return c.links.FindByID(ctx, id)
}

// Proposition devolve uma Proposição pelo id.
func (c *Catalog) Proposition(ctx context.Context, id string) (domain.Proposition, error) {
	return c.props.FindByID(ctx, id)
}

// ConfirmedLinkIDs lista os Vínculos que a Sincronização periódica cobre.
func (c *Catalog) ConfirmedLinkIDs(ctx context.Context) ([]string, error) {
	return c.links.ListConfirmedIDs(ctx)
}

// Candidate é um registro de outra Casa que pode ser a mesma Proposição.
type Candidate struct {
	Chamber    domain.Chamber
	ExternalID string
	SourceName string
	SourceURL  string
}

// CreateImportedInput é a Proposição escolhida pelo Editor numa Casa.
type CreateImportedInput struct {
	Chamber       domain.Chamber
	ExternalID    string
	SourceName    string
	SourceURL     string
	Kind          string
	Number        int
	Year          int
	OriginChamber domain.Chamber
	Data          domain.OfficialData
	LatestEventAt *time.Time
	// EditorID é quem importou: escolher o registro já é confirmar o
	// Vínculo dessa Casa.
	EditorID string
	// Candidates viram Vínculos sugeridos, nunca confirmados.
	Candidates []Candidate
}

// CreateImportedResult é o que foi criado.
type CreateImportedResult struct {
	Proposition domain.Proposition
	Link        domain.SourceLink
	Suggested   []domain.SourceLink
}

// CreateImported cria a Proposição em "criado" com a Versão 1 em análise, o Vínculo confirmado da
// Casa escolhida e os sugeridos das outras, e resolve a Apensação nos dois
// sentidos. Chame dentro de uma transação.
func (c *Catalog) CreateImported(ctx context.Context, in CreateImportedInput) (CreateImportedResult, error) {
	now := c.clock.Now()
	origin := in.OriginChamber
	if _, err := domain.ParseChamber(string(origin)); err != nil {
		// A fonte não disse quem iniciou: a Casa de onde importamos.
		origin = in.Chamber
	}
	kind, err := domain.ParseKind(in.Kind)
	if err != nil {
		return CreateImportedResult{}, err
	}
	slug, err := c.freeSlug(ctx, kind, in.Number, in.Year, origin)
	if err != nil {
		return CreateImportedResult{}, err
	}
	id, err := newID()
	if err != nil {
		return CreateImportedResult{}, err
	}
	p, err := domain.NewProposition(id, slug, kind, in.Number, in.Year, origin, in.Data, now)
	if err != nil {
		return CreateImportedResult{}, err
	}
	if in.LatestEventAt != nil {
		t := in.LatestEventAt.UTC()
		p.StatusAt = &t
	}
	if err := c.resolvePrincipal(ctx, &p); err != nil {
		return CreateImportedResult{}, err
	}
	if err := c.props.Create(ctx, p); err != nil {
		return CreateImportedResult{}, err
	}
	// A Versão 1 ("Texto original") nasce em análise, com conteúdo vazio:
	// é nela que o Editor escreve o primeiro Resumo.
	versionID, err := newID()
	if err != nil {
		return CreateImportedResult{}, err
	}
	v := domain.NewImportedVersion(versionID, p, now)
	if err := c.versions.Create(ctx, v); err != nil {
		return CreateImportedResult{}, err
	}
	if err := c.contents.Create(ctx, domain.NewEmptyContent(v.ID, now)); err != nil {
		return CreateImportedResult{}, err
	}
	// O PDF oficial é buscado e lido em segundo plano (Estruturação).
	if err := startStructuring(ctx, c.structurings, c.sched, v, now); err != nil {
		return CreateImportedResult{}, err
	}

	linkID, err := newID()
	if err != nil {
		return CreateImportedResult{}, err
	}
	link, err := domain.NewConfirmedLink(linkID, p.ID, in.Chamber, in.ExternalID, in.SourceName, in.SourceURL, in.EditorID, now)
	if err != nil {
		return CreateImportedResult{}, err
	}
	link.SyncSucceeded(now) // a Importação já trouxe tudo desta Casa
	if err := c.links.Create(ctx, link); err != nil {
		return CreateImportedResult{}, err
	}

	res := CreateImportedResult{Proposition: p, Link: link}
	for _, cand := range in.Candidates {
		l, ok, err := c.suggest(ctx, p.ID, in.Chamber, cand, now)
		if err != nil {
			return CreateImportedResult{}, err
		}
		if ok {
			res.Suggested = append(res.Suggested, l)
		}
	}
	if err := c.resolveAttached(ctx, p, now); err != nil {
		return CreateImportedResult{}, err
	}
	return res, nil
}

// freeSlug escolhe o identificador público: "pec-221-2019" ou, se já
// existir (outra Casa iniciadora com a mesma numeração),
// "pec-221-2019-senado".
func (c *Catalog) freeSlug(ctx context.Context, kind string, number, year int, origin domain.Chamber) (string, error) {
	for _, s := range []string{domain.Slug(kind, number, year), domain.SlugWithChamber(kind, number, year, origin)} {
		taken, err := c.props.SlugExists(ctx, s)
		if err != nil {
			return "", err
		}
		if !taken {
			return s, nil
		}
	}
	return "", domain.ErrSlugTaken
}

// suggest grava um candidato como Vínculo sugerido, a menos que o registro
// já esteja confirmado em alguma Proposição ou seja da Casa importada.
func (c *Catalog) suggest(ctx context.Context, propositionID string, imported domain.Chamber, cand Candidate, now time.Time) (domain.SourceLink, bool, error) {
	if cand.Chamber == imported {
		return domain.SourceLink{}, false, nil
	}
	existing, err := c.links.ListByExternal(ctx, cand.Chamber, []string{cand.ExternalID})
	if err != nil {
		return domain.SourceLink{}, false, err
	}
	for _, e := range existing {
		if e.State == domain.LinkConfirmed || e.PropositionID == propositionID {
			return domain.SourceLink{}, false, nil
		}
	}
	id, err := newID()
	if err != nil {
		return domain.SourceLink{}, false, err
	}
	l, err := domain.NewSuggestedLink(id, propositionID, cand.Chamber, cand.ExternalID, cand.SourceName, cand.SourceURL, now)
	if err != nil {
		// Candidato malformado vindo da fonte: só não vira sugestão.
		return domain.SourceLink{}, false, nil
	}
	if err := c.links.Create(ctx, l); err != nil {
		if errors.Is(err, domain.ErrLinkExists) {
			return domain.SourceLink{}, false, nil
		}
		return domain.SourceLink{}, false, err
	}
	return l, true, nil
}

// resolvePrincipal liga p à principal pelo nome oficial que a fonte deu,
// se ela estiver cadastrada e o nome for inequívoco.
func (c *Catalog) resolvePrincipal(ctx context.Context, p *domain.Proposition) error {
	if p.AttachedToName == "" || p.AttachedToID != nil {
		return nil
	}
	kind, number, year, err := domain.ParseOfficialName(p.AttachedToName)
	if err != nil {
		return nil // nome que não sabemos ler: fica só o texto
	}
	found, err := c.props.FindByName(ctx, kind, number, year)
	if err != nil {
		return err
	}
	if len(found) == 1 {
		p.AttachTo(found[0].ID)
	}
	return nil
}

// resolveAttached liga a p as apensadas que esperavam por ela.
func (c *Catalog) resolveAttached(ctx context.Context, p domain.Proposition, now time.Time) error {
	same, err := c.props.FindByName(ctx, p.Kind, p.Number, p.Year)
	if err != nil {
		return err
	}
	if len(same) != 1 {
		return nil // nome ambíguo entre Casas: não adivinha
	}
	waiting, err := c.props.ListUnresolvedAttachments(ctx, p.Name())
	if err != nil {
		return err
	}
	for _, a := range waiting {
		if a.ID == p.ID {
			continue
		}
		a.AttachTo(p.ID)
		a.UpdatedAt = now
		if err := c.props.Update(ctx, a); err != nil {
			return fmt.Errorf("resolvendo apensação de %s: %w", a.ID, err)
		}
	}
	return nil
}

// ApplySyncInput é o resultado de buscar um Vínculo na fonte.
type ApplySyncInput struct {
	LinkID        string
	Data          domain.OfficialData
	LatestEventAt *time.Time
	SourceName    string
	SourceURL     string
	// ReportsAttachment: a fonte desta Casa informa a Apensação (ver
	// domain.SyncUpdate).
	ReportsAttachment bool
}

// ApplySync aplica uma Sincronização bem-sucedida: dados oficiais na
// Proposição (sem tocar no estado de Revisão) e data da última
// Sincronização no Vínculo. Chame dentro de uma transação.
func (c *Catalog) ApplySync(ctx context.Context, in ApplySyncInput) error {
	link, err := c.links.FindByID(ctx, in.LinkID)
	if err != nil {
		return err
	}
	if link.State != domain.LinkConfirmed {
		return domain.ErrLinkNotFound // sugestão não sincroniza
	}
	p, err := c.props.FindByIDForUpdate(ctx, link.PropositionID)
	if err != nil {
		return err
	}
	now := c.clock.Now()
	p.ApplySync(domain.SyncUpdate{
		Chamber: link.Chamber, Data: in.Data, LatestEventAt: in.LatestEventAt, ReportsAttachment: in.ReportsAttachment,
	}, now)
	if err := c.resolvePrincipal(ctx, &p); err != nil {
		return err
	}
	if err := c.props.Update(ctx, p); err != nil {
		return err
	}
	if err := c.resolveAttached(ctx, p, now); err != nil {
		return err
	}
	link.UpdateSource(in.SourceName, in.SourceURL)
	link.SyncSucceeded(now)
	return c.links.UpdateSync(ctx, link)
}

// RecordSyncFailure guarda, só para a equipe, que a última Sincronização
// falhou.
func (c *Catalog) RecordSyncFailure(ctx context.Context, linkID, msg string) error {
	link, err := c.links.FindByID(ctx, linkID)
	if err != nil {
		return err
	}
	link.SyncFailed(msg)
	return c.links.UpdateSync(ctx, link)
}
