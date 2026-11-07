package main

import (
	"fmt"

	ingestapp "github.com/mphennrichs/nelisa/api/internal/ingest/app"
	"github.com/mphennrichs/nelisa/api/internal/ingest/sources/camara"
	"github.com/mphennrichs/nelisa/api/internal/ingest/sources/senado"
)

// newSources monta as fontes oficiais com API (Câmara e Senado), com as
// URLs reais e os limites padrão dos adaptadores (timeout, concorrência por
// domínio, tamanho máximo de resposta). Cada processo (api, worker) tem as
// suas instâncias, e o limite de concorrência vale por processo.
func newSources() (ingestapp.Sources, error) {
	c, err := camara.New(nil, "", camara.Config{})
	if err != nil {
		return ingestapp.Sources{}, fmt.Errorf("montando fonte da câmara: %w", err)
	}
	s, err := senado.New(nil, "", senado.Config{})
	if err != nil {
		return ingestapp.Sources{}, fmt.Errorf("montando fonte do senado: %w", err)
	}
	return ingestapp.NewSources(c, s), nil
}
