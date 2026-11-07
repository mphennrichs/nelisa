// Package app tem os casos de uso de Importação e Sincronização e as portas
// que eles usam. Este arquivo é o contrato com as fontes oficiais (APIs da
// Câmara e do Senado): os adaptadores em internal/ingest/sources/* o
// implementam; os casos de uso só conhecem esta interface.
package app

import (
	"context"
	"errors"
	"time"
)

// ChamberCode identifica a Casa legislativa de uma fonte com API.
type ChamberCode string

const (
	ChamberCamara ChamberCode = "camara"
	ChamberSenado ChamberCode = "senado"
)

// SearchQuery busca uma Proposição pelo nome oficial ("PEC 221/2019").
type SearchQuery struct {
	Kind   string // sigla do Tipo em maiúsculas: PL, PEC, PLP, MPV...
	Number int
	Year   int
}

// SourceRecord é a Proposição como uma Casa a descreve. Tudo aqui é dado
// oficial; nada é produzido pelo Nelisa.
type SourceRecord struct {
	Chamber    ChamberCode
	ExternalID string // ID da Proposição na API da Casa (string: cada Casa tem seu formato)
	Kind       string
	Number     int
	Year       int
	// Ementa oficial.
	Summary     string
	PresentedAt time.Time
	// OfficialTextURL é o link do Documento integral na Casa ("" se não houver).
	OfficialTextURL string
	// Authors como a Casa escreve ("Câmara dos Deputados", "Dep. Fulano (PT/SP)").
	Authors []string
	// Role da Casa nesta Proposição: "origin" (iniciadora) ou "review"
	// (revisora); "" se a fonte não informar.
	Role string
	// OriginChamber é a Casa iniciadora quando a fonte informa ("" se não).
	OriginChamber ChamberCode
	// Status é a situação atual como a Casa descreve.
	Status string
	// AttachedTo é o nome oficial da Proposição principal se esta estiver
	// apensada ("PEC 221/2019"); "" caso contrário.
	AttachedTo string
	// SourceURL é a página pública da Proposição na Casa, para o Leitor
	// conferir.
	SourceURL string
}

// SourceProgress é um evento de Tramitação numa Casa.
type SourceProgress struct {
	// ExternalKey identifica o evento na fonte de forma estável (ex.: número
	// de sequência da Câmara) — a Sincronização usa para não duplicar.
	ExternalKey string
	At          time.Time
	Body        string // órgão (sigla), ex.: "PLEN", "CCJ", "MESA"
	Description string // o que aconteceu
	Situation   string // situação depois do evento ("" se não houver)
	Dispatch    string // despacho/texto oficial
}

// ErrNotFound: a fonte não tem a Proposição pedida.
var ErrNotFound = errors.New("proposição não encontrada na fonte")

// ErrUnavailable: a fonte falhou (rede, timeout, 5xx, resposta inválida).
// Quem chama tenta de novo depois; nunca grava dado parcial.
var ErrUnavailable = errors.New("fonte oficial indisponível")

// Source é uma fonte oficial com API. Implementações devem: usar timeout,
// User-Agent identificando o Nelisa, limitar a concorrência por domínio,
// tratar toda resposta como hostil (validar campos e tamanhos) e devolver
// ErrNotFound/ErrUnavailable (embrulhados) nos casos acima.
type Source interface {
	Chamber() ChamberCode
	// Search devolve os registros que batem com o nome oficial (normalmente
	// 0 ou 1). Na Casa revisora o nome pode ser o mesmo da iniciadora.
	Search(ctx context.Context, q SearchQuery) ([]SourceRecord, error)
	// Fetch devolve um registro pelo ID externo.
	Fetch(ctx context.Context, externalID string) (SourceRecord, error)
	// Progress devolve a Tramitação completa na Casa, do mais antigo ao
	// mais novo.
	Progress(ctx context.Context, externalID string) ([]SourceProgress, error)
}
