// Package officialtext busca o PDF oficial de uma Versão e extrai o texto
// de cada página (adaptador da porta app.OfficialText). Só fala com Fonte
// oficial com API (domain.OfficialTextFetchable), inclusive em cada
// redirecionamento; o PDF não é guardado.
package officialtext

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/mphennrichs/nelisa/api/internal/proposition/app"
	"github.com/mphennrichs/nelisa/api/internal/proposition/domain"
)

const (
	// MaxPDFBytes: os PDFs reais têm de 30 KB a 500 KB; 20 MB sobra.
	MaxPDFBytes   = 20 << 20
	fetchTimeout  = 60 * time.Second
	maxRedirects  = 3
	userAgent     = "Nelisa/1.0 (+estruturacao)"
	pdfMagicRange = 1024
)

// Extractor lê o texto de cada página de um PDF.
type Extractor interface {
	Pages(ctx context.Context, pdf []byte) ([]string, error)
}

// Fetcher implementa app.OfficialText.
type Fetcher struct {
	client    *http.Client
	extractor Extractor
}

var errHostNotAllowed = errors.New("redirecionamento para host fora das Fontes oficiais")

// New monta o Fetcher. transport nil usa o padrão (testes passam o de um
// httptest.Server).
func New(extractor Extractor, transport http.RoundTripper) *Fetcher {
	c := &http.Client{
		Timeout:   fetchTimeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > maxRedirects {
				return fmt.Errorf("mais de %d redirecionamentos", maxRedirects)
			}
			if !domain.OfficialTextFetchable(req.URL.String()) {
				return errHostNotAllowed
			}
			return nil
		},
	}
	return &Fetcher{client: c, extractor: extractor}
}

func fail(f domain.StructuringFailure, temporary bool, err error) error {
	return &app.TextFetchError{Failure: f, Temporary: temporary, Err: err}
}

func (f *Fetcher) Fetch(ctx context.Context, url string) ([]string, error) {
	if !domain.OfficialTextFetchable(url) {
		return nil, fail(domain.FailureHostNotAllowed, false, fmt.Errorf("link fora das Fontes oficiais"))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fail(domain.FailureHostNotAllowed, false, err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/pdf")
	resp, err := f.client.Do(req)
	if errors.Is(err, errHostNotAllowed) {
		return nil, fail(domain.FailureHostNotAllowed, false, err)
	}
	if err != nil {
		return nil, fail(domain.FailureNetwork, true, err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
		return nil, fail(domain.FailureNetwork, true, fmt.Errorf("status %d", resp.StatusCode))
	case resp.StatusCode != http.StatusOK:
		return nil, fail(domain.FailureNotFound, false, fmt.Errorf("status %d", resp.StatusCode))
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxPDFBytes+1))
	if err != nil {
		return nil, fail(domain.FailureNetwork, true, err)
	}
	if len(body) > MaxPDFBytes {
		return nil, fail(domain.FailureTooLarge, false, fmt.Errorf("mais de %d bytes", MaxPDFBytes))
	}
	if !bytes.Contains(body[:min(len(body), pdfMagicRange)], []byte("%PDF-")) {
		return nil, fail(domain.FailureNotPDF, false, fmt.Errorf("conteúdo não é PDF (%s)", resp.Header.Get("Content-Type")))
	}
	pages, err := f.extractor.Pages(ctx, body)
	if err != nil {
		// PDF corrompido ou cifrado: tentar de novo não muda nada.
		return nil, fail(domain.FailureNotPDF, false, err)
	}
	return pages, nil
}
