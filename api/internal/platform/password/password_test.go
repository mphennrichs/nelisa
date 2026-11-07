package password

import "testing"

func TestBcrypt(t *testing.T) {
	b := Bcrypt{cost: 4} // custo mínimo: o teste confere o contrato, não a força
	h, err := b.Hash("senha-certa")
	if err != nil {
		t.Fatal(err)
	}
	if h == "senha-certa" || !b.Verify("senha-certa", h) || b.Verify("senha-errada", h) || b.Verify("senha-certa", "lixo") {
		t.Fatal("bcrypt contract broken")
	}
}
