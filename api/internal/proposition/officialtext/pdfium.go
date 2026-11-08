package officialtext

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/klippa-app/go-pdfium"
	"github.com/klippa-app/go-pdfium/requests"
	"github.com/klippa-app/go-pdfium/webassembly"
)

// PDFium extrai texto com o PDFium compilado para WebAssembly (wazero):
// sem cgo, e o PDF, que vem de fora, é lido dentro da sandbox de memória
// do WebAssembly. O runtime sobe na primeira extração (o modo api nunca
// extrai) e as extrações são uma de cada vez.
type PDFium struct {
	mu   sync.Mutex
	pool pdfium.Pool
	inst pdfium.Pdfium
}

func NewPDFium() *PDFium { return &PDFium{} }

func (p *PDFium) instance() (pdfium.Pdfium, error) {
	if p.inst != nil {
		return p.inst, nil
	}
	pool, err := webassembly.Init(webassembly.Config{MinIdle: 1, MaxIdle: 1, MaxTotal: 1})
	if err != nil {
		return nil, fmt.Errorf("iniciando pdfium: %w", err)
	}
	inst, err := pool.GetInstance(30 * time.Second)
	if err != nil {
		_ = pool.Close()
		return nil, fmt.Errorf("instância do pdfium: %w", err)
	}
	p.pool, p.inst = pool, inst
	return inst, nil
}

func (p *PDFium) Pages(_ context.Context, pdf []byte) ([]string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	inst, err := p.instance()
	if err != nil {
		return nil, err
	}
	doc, err := inst.OpenDocument(&requests.OpenDocument{File: &pdf})
	if err != nil {
		return nil, fmt.Errorf("abrindo pdf: %w", err)
	}
	defer func() { _, _ = inst.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: doc.Document}) }()
	cnt, err := inst.FPDF_GetPageCount(&requests.FPDF_GetPageCount{Document: doc.Document})
	if err != nil {
		return nil, fmt.Errorf("contando páginas: %w", err)
	}
	pages := make([]string, 0, cnt.PageCount)
	for i := 0; i < cnt.PageCount; i++ {
		t, err := inst.GetPageText(&requests.GetPageText{
			Page: requests.Page{ByIndex: &requests.PageByIndex{Document: doc.Document, Index: i}},
		})
		if err != nil {
			return nil, fmt.Errorf("lendo página %d: %w", i+1, err)
		}
		pages = append(pages, t.Text)
	}
	return pages, nil
}

// Close desliga o runtime (fim do worker).
func (p *PDFium) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pool == nil {
		return nil
	}
	if p.inst != nil {
		_ = p.inst.Close()
	}
	err := p.pool.Close()
	p.pool, p.inst = nil, nil
	return err
}
