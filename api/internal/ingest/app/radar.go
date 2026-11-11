package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"time"

	"github.com/mphennrichs/nelisa/api/internal/platform/clock"
)

// Radar (#45): uma vez por dia o worker procura nas Casas as Proposições em
// pauta no Plenário e as recém-apresentadas, e a equipe vê o que ainda não
// está no Nelisa. O Radar não importa nada: quem importa é o Editor, pelo
// fluxo de Importação (CONTEXT.md, "Importação" e "Radar").

// RadarKinds são os Tipos que o Radar acompanha.
var RadarKinds = []string{"PL", "PEC", "PLP"}

const (
	// RadarAgendaDays: a pauta é lida de hoje até tantos dias à frente.
	RadarAgendaDays = 7
	// RadarPresentedDays: as apresentações são lidas dos últimos dias (um
	// dia sem rodar não perde nada).
	RadarPresentedDays = 3
	// radarPresentedShown: por quantos dias uma apresentação fica na lista.
	radarPresentedShown = 30 * 24 * time.Hour
	// radarAgendaGrace: a pauta continua na lista até um dia depois da
	// sessão.
	radarAgendaGrace = 24 * time.Hour
	// radarRetention: o que a fonte não traz há tanto tempo é apagado.
	radarRetention = 60 * 24 * time.Hour
)

// ErrRadarItemNotFound: o item não existe (ou já foi apagado).
var ErrRadarItemNotFound = errors.New("ingest: item do radar não encontrado")

// RadarHit é uma Proposição achada numa fonte: na pauta de uma sessão do
// Plenário (AgendaAt) ou apresentada (PresentedAt).
type RadarHit struct {
	Chamber     ChamberCode
	ExternalID  string
	Kind        string
	Number      int
	Year        int
	Summary     string
	SourceURL   string
	AgendaAt    *time.Time
	PresentedAt *time.Time
}

// RadarSource é a parte de uma fonte que alimenta o Radar.
type RadarSource interface {
	Chamber() ChamberCode
	// Agenda devolve as Proposições de RadarKinds na pauta do Plenário
	// entre from e to (dias inteiros, horário de Brasília).
	Agenda(ctx context.Context, from, to time.Time) ([]RadarHit, error)
	// Presented devolve as de RadarKinds apresentadas entre from e to.
	Presented(ctx context.Context, from, to time.Time) ([]RadarHit, error)
}

// RadarItem é um item guardado do Radar.
type RadarItem struct {
	RadarHit
	FirstSeenAt time.Time
	DismissedAt *time.Time
}

// RadarRepo guarda o Radar.
type RadarRepo interface {
	// Upsert grava os achados: a pauta mais recente e a primeira data de
	// apresentação ficam; a dispensa não muda.
	Upsert(ctx context.Context, hits []RadarHit, now time.Time) error
	// Visible lista o que a equipe deve ver: pauta a partir de agendaSince
	// ou apresentação a partir de presentedSince, sem os dispensados (que
	// voltam com pauta nova).
	Visible(ctx context.Context, agendaSince, presentedSince time.Time) ([]RadarItem, error)
	// Dismiss marca dispensado; ErrRadarItemNotFound se não há.
	Dismiss(ctx context.Context, chamber ChamberCode, externalID, by string, now time.Time) (RadarItem, error)
	// Purge apaga o que a fonte não traz desde before.
	Purge(ctx context.Context, before time.Time) (int, error)
	// LastSeen é quando a última rodada achou algo (nil se nunca).
	LastSeen(ctx context.Context) (*time.Time, error)
}

// RadarScheduler enfileira uma rodada do Radar (River, ligado no main).
// Rodada já na fila não ganha outra.
type RadarScheduler interface {
	ScheduleRadarScan(ctx context.Context) error
}

func (s Sources) radars() []RadarSource {
	var out []RadarSource
	for _, c := range []ChamberCode{ChamberCamara, ChamberSenado} {
		if r, ok := s.byChamber[c].(RadarSource); ok {
			out = append(out, r)
		}
	}
	return out
}

// ScanRadarResult resume uma rodada.
type ScanRadarResult struct {
	Found  int
	Purged int
}

// ScanRadar é a rodada diária. Operação de sistema (job do worker). Uma
// Casa fora do ar não impede a outra: o que veio é gravado e o erro volta
// no fim, para o job tentar de novo.
type ScanRadar struct {
	sources Sources
	repo    RadarRepo
	clock   clock.Clock
}

func NewScanRadar(s Sources, r RadarRepo, clk clock.Clock) *ScanRadar {
	return &ScanRadar{sources: s, repo: r, clock: clk}
}

func (uc *ScanRadar) Execute(ctx context.Context) (ScanRadarResult, error) {
	now := uc.clock.Now()
	today := now.In(brasilia)
	var res ScanRadarResult
	var errs []error
	for _, src := range uc.sources.radars() {
		agenda, err := src.Agenda(ctx, today, today.AddDate(0, 0, RadarAgendaDays))
		if err != nil {
			errs = append(errs, fmt.Errorf("pauta %s: %w", src.Chamber(), err))
		}
		presented, err := src.Presented(ctx, today.AddDate(0, 0, -RadarPresentedDays), today)
		if err != nil {
			errs = append(errs, fmt.Errorf("apresentadas %s: %w", src.Chamber(), err))
		}
		hits := validHits(src.Chamber(), append(agenda, presented...))
		if len(hits) == 0 {
			continue
		}
		if err := uc.repo.Upsert(ctx, hits, now); err != nil {
			return res, err
		}
		res.Found += len(hits)
	}
	n, err := uc.repo.Purge(ctx, now.Add(-radarRetention))
	if err != nil {
		return res, err
	}
	res.Purged = n
	return res, errors.Join(errs...)
}

// validHits descarta o que não é da Casa, de outro Tipo ou sem nome e
// junta o mesmo registro achado duas vezes (pauta e apresentação).
func validHits(c ChamberCode, hits []RadarHit) []RadarHit {
	byID := map[string]int{}
	var out []RadarHit
	for _, h := range hits {
		if h.Chamber != c || h.ExternalID == "" || !slices.Contains(RadarKinds, h.Kind) || h.Number <= 0 || h.Year <= 0 {
			continue
		}
		i, seen := byID[h.ExternalID]
		if !seen {
			byID[h.ExternalID] = len(out)
			out = append(out, h)
			continue
		}
		o := &out[i]
		if h.AgendaAt != nil && (o.AgendaAt == nil || h.AgendaAt.After(*o.AgendaAt)) {
			o.AgendaAt = h.AgendaAt
		}
		if o.PresentedAt == nil {
			o.PresentedAt = h.PresentedAt
		}
		if o.Summary == "" {
			o.Summary = h.Summary
		}
		if o.SourceURL == "" {
			o.SourceURL = h.SourceURL
		}
	}
	return out
}

var brasilia = func() *time.Location {
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		return time.FixedZone("BRT", -3*60*60)
	}
	return loc
}()

// ListRadar: a equipe vê o Radar, só com o que ainda não está no Nelisa
// (nem como Vínculo sugerido). Primeiro a pauta, da sessão mais próxima;
// depois as apresentações, das mais novas.
type ListRadar struct {
	repo    RadarRepo
	catalog Catalog
	clock   clock.Clock
}

func NewListRadar(r RadarRepo, c Catalog, clk clock.Clock) *ListRadar {
	return &ListRadar{repo: r, catalog: c, clock: clk}
}

// RadarView é a lista e quando o Radar achou algo pela última vez.
type RadarView struct {
	Items    []RadarItem
	LastSeen *time.Time
}

func (uc *ListRadar) Execute(ctx context.Context) (RadarView, error) {
	if _, err := requireEditor(ctx); err != nil {
		return RadarView{}, err
	}
	last, err := uc.repo.LastSeen(ctx)
	if err != nil {
		return RadarView{}, err
	}
	now := uc.clock.Now()
	items, err := uc.repo.Visible(ctx, now.Add(-radarAgendaGrace), now.Add(-radarPresentedShown))
	if err != nil {
		return RadarView{}, err
	}
	ids := map[ChamberCode][]string{}
	for _, it := range items {
		ids[it.Chamber] = append(ids[it.Chamber], it.ExternalID)
	}
	known := map[ChamberCode]map[string]bool{}
	for c, list := range ids {
		links, err := uc.catalog.LinksByExternal(ctx, c, list)
		if err != nil {
			return RadarView{}, err
		}
		known[c] = map[string]bool{}
		for _, l := range links {
			known[c][l.ExternalID] = true
		}
	}
	out := make([]RadarItem, 0, len(items))
	for _, it := range items {
		if !known[it.Chamber][it.ExternalID] {
			out = append(out, it)
		}
	}
	sortRadar(out, now.Add(-radarAgendaGrace))
	return RadarView{Items: out, LastSeen: last}, nil
}

// sortRadar: pauta futura (ou de ontem) primeiro, da mais próxima; depois
// por apresentação, da mais nova.
func sortRadar(items []RadarItem, agendaSince time.Time) {
	onAgenda := func(it RadarItem) bool { return it.AgendaAt != nil && !it.AgendaAt.Before(agendaSince) }
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if onAgenda(a) != onAgenda(b) {
			return onAgenda(a)
		}
		if onAgenda(a) && !a.AgendaAt.Equal(*b.AgendaAt) {
			return a.AgendaAt.Before(*b.AgendaAt)
		}
		pa, pb := a.PresentedAt, b.PresentedAt
		if pa != nil && pb != nil && !pa.Equal(*pb) {
			return pa.After(*pb)
		}
		if (pa == nil) != (pb == nil) {
			return pa != nil
		}
		if a.Chamber != b.Chamber {
			return a.Chamber < b.Chamber
		}
		return a.ExternalID < b.ExternalID
	})
}

// ActionRadarDismiss: um Editor dispensou um item do Radar.
const ActionRadarDismiss = "radar.dismiss"

// DismissRadarItem: o Editor tira um item do Radar. Volta se entrar numa
// pauta posterior.
type DismissRadarItem struct {
	repo  RadarRepo
	tx    TxRunner
	audit AuditRecorder
	clock clock.Clock
}

func NewDismissRadarItem(r RadarRepo, tx TxRunner, a AuditRecorder, clk clock.Clock) *DismissRadarItem {
	return &DismissRadarItem{repo: r, tx: tx, audit: a, clock: clk}
}

func (uc *DismissRadarItem) Execute(ctx context.Context, chamber ChamberCode, externalID string) error {
	a, err := requireEditor(ctx)
	if err != nil {
		return err
	}
	return uc.tx.WithinTx(ctx, func(ctx context.Context) error {
		it, err := uc.repo.Dismiss(ctx, chamber, externalID, a.UserID, uc.clock.Now())
		if err != nil {
			return err
		}
		return uc.audit.Record(ctx, AuditEvent{
			ActorID: a.UserID, Action: ActionRadarDismiss, TargetType: "radar_item",
			TargetID: string(it.Chamber) + ":" + it.ExternalID,
			Metadata: map[string]any{"name": fmt.Sprintf("%s %d/%d", it.Kind, it.Number, it.Year)},
		})
	})
}

// RequestRadarScan: o Editor pede uma rodada agora (por exemplo, logo
// depois do deploy, sem esperar a das 6h).
type RequestRadarScan struct{ sched RadarScheduler }

func NewRequestRadarScan(s RadarScheduler) *RequestRadarScan { return &RequestRadarScan{sched: s} }

func (uc *RequestRadarScan) Execute(ctx context.Context) error {
	if _, err := requireEditor(ctx); err != nil {
		return err
	}
	return uc.sched.ScheduleRadarScan(ctx)
}
