package app

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/mphennrichs/nelisa/api/internal/platform/auth"
)

var (
	// ErrUnauthenticated: sem Ator no contexto (Leitor anônimo).
	ErrUnauthenticated = errors.New("ingest: não autenticado")
	// ErrForbidden: o Ator não tem a role que a ação exige.
	ErrForbidden = errors.New("ingest: sem permissão")
	// ErrUnknownChamber: nenhuma fonte configurada para essa Casa.
	ErrUnknownChamber = errors.New("ingest: casa sem fonte configurada")
	ErrInvalidQuery   = errors.New("ingest: busca inválida (tipo, número e ano)")
	ErrInvalidID      = errors.New("ingest: id externo inválido")
	// ErrInvalidRecord: a fonte devolveu um registro que não serve (outro
	// id, sem Tipo/número/ano). Tratado como fonte indisponível.
	ErrInvalidRecord = errors.New("ingest: registro inválido na fonte")
	// ErrAlreadySuggested: o registro já é sugestão de Vínculo de uma
	// Proposição; o Editor deve confirmá-lo lá, não importar de novo.
	ErrAlreadySuggested = errors.New("ingest: registro já sugerido para uma proposição")
	// ErrPropositionExists: já existe Proposição com a mesma Casa
	// iniciadora, Tipo, número e ano, ligada a outro registro.
	ErrPropositionExists = errors.New("ingest: proposição já cadastrada")
	// ErrLinkTaken: o registro foi confirmado em outra Proposição
	// enquanto importávamos.
	ErrLinkTaken = errors.New("ingest: registro já vinculado")
	// ErrLinkNotFound: o Vínculo não existe mais.
	ErrLinkNotFound = errors.New("ingest: vínculo não encontrado")
)

// LinkRef é um Vínculo de fonte como o ingest precisa vê-lo.
type LinkRef struct {
	ID            string
	PropositionID string
	Chamber       ChamberCode
	ExternalID    string
	Confirmed     bool
}

// Candidate é um registro de outra Casa que pode ser a mesma Proposição.
type Candidate struct {
	Chamber    ChamberCode
	ExternalID string
	SourceName string
	SourceURL  string
}

// ImportRequest é o que a Importação manda criar.
type ImportRequest struct {
	Record        SourceRecord
	LatestEventAt *time.Time
	EditorID      string
	Candidates    []Candidate
}

// Imported é o que foi criado.
type Imported struct {
	PropositionID string
	Name          string
	Suggested     int
}

// SyncResult é o que uma Sincronização bem-sucedida aplica.
type SyncResult struct {
	LinkID        string
	Record        SourceRecord
	LatestEventAt *time.Time
}

// Catalog é a parte do módulo proposition que o ingest usa (ligado no
// main). Não autoriza nem abre transação: o caso de uso do ingest faz isso.
// Erros: ErrLinkNotFound, ErrLinkTaken, ErrPropositionExists,
// ErrInvalidRecord.
type Catalog interface {
	LinksByExternal(ctx context.Context, chamber ChamberCode, externalIDs []string) ([]LinkRef, error)
	// CreateImported cria a Proposição em "criado", o Vínculo confirmado
	// da Casa importada e os sugeridos dos candidatos.
	CreateImported(ctx context.Context, in ImportRequest) (Imported, error)
	Link(ctx context.Context, id string) (LinkRef, error)
	ConfirmedLinkIDs(ctx context.Context) ([]string, error)
	ApplySync(ctx context.Context, in SyncResult) error
	RecordSyncFailure(ctx context.Context, linkID, msg string) error
}

// ProgressWriter grava a Tramitação de uma Casa (módulo progress, ligado
// no main), sem duplicar eventos já gravados.
type ProgressWriter interface {
	Record(ctx context.Context, propositionID string, chamber ChamberCode, entries []SourceProgress) (inserted int, err error)
}

// SyncScheduler enfileira Sincronizações (River, ligado no main).
type SyncScheduler interface {
	ScheduleSyncs(ctx context.Context, linkIDs []string) error
}

// TxRunner abre uma transação levada no contexto.
type TxRunner interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// AuditEvent é o que o módulo manda para a Auditoria.
type AuditEvent struct {
	ActorID    string
	Action     string
	TargetType string
	TargetID   string
	Metadata   map[string]any
}

// AuditRecorder grava um evento de Auditoria na transação do contexto.
type AuditRecorder interface {
	Record(ctx context.Context, e AuditEvent) error
}

// ReportsAttachment diz se a fonte da Casa informa a Apensação
// (SourceRecord.AttachedTo). A Câmara informa enquanto a Proposição está
// apensada e devolve "" quando deixa de estar; o Senado nunca informa, e o
// "" dele não quer dizer "não apensada". Por isso só a Câmara dirige a
// Apensação.
func ReportsAttachment(c ChamberCode) bool { return c == ChamberCamara }

// ActionImport é a Importação de uma Proposição por um Editor.
const ActionImport = "proposition.import"

// Sources indexa as fontes configuradas por Casa.
type Sources struct{ byChamber map[ChamberCode]Source }

func NewSources(list ...Source) Sources {
	s := Sources{byChamber: map[ChamberCode]Source{}}
	for _, src := range list {
		if src != nil {
			s.byChamber[src.Chamber()] = src
		}
	}
	return s
}

func (s Sources) get(c ChamberCode) (Source, bool) {
	src, ok := s.byChamber[c]
	return src, ok
}

// chambers devolve as Casas configuradas em ordem estável.
func (s Sources) chambers() []ChamberCode {
	out := make([]ChamberCode, 0, len(s.byChamber))
	for c := range s.byChamber {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func requireEditor(ctx context.Context) (auth.Actor, error) {
	a, ok := auth.ActorFrom(ctx)
	if !ok {
		return auth.Actor{}, ErrUnauthenticated
	}
	if !a.Role.AtLeast(auth.RoleEditor) {
		return a, ErrForbidden
	}
	return a, nil
}

// latestAt é a data do evento mais recente (nil se não há eventos).
func latestAt(ps []SourceProgress) *time.Time {
	var out *time.Time
	for _, p := range ps {
		if !p.At.IsZero() && (out == nil || p.At.After(*out)) {
			t := p.At
			out = &t
		}
	}
	return out
}
