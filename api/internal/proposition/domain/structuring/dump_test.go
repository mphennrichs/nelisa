package structuring

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// go test -run TestDump -dump=testdata/x.pages.txt mostra a árvore.
func TestDump(t *testing.T) {
	f := os.Getenv("DUMP")
	if f == "" {
		t.Skip()
	}
	b, _ := os.ReadFile(f)
	r := Parse(Clean(strings.Split(string(b), "\f")))
	for _, p := range r.Provisions {
		txt := []rune(p.Text)
		if len(txt) > 90 {
			txt = append(txt[:90], '…')
		}
		fmt.Printf("%s%-28s %s\n", strings.Repeat("  ", p.Depth), p.Address, string(txt))
	}
	fmt.Println(r.Warnings)
}
