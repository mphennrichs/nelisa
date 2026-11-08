package app

import (
	"context"
	"slices"
)

// Person é alguém da equipe como os outros módulos o veem: nome e se a
// conta está ativa. Nunca e-mail, role ou senha.
type Person struct {
	ID     string
	Name   string
	Active bool
}

// Directory é a operação de sistema que os outros módulos usam (ligada no
// main) para mostrar nomes da equipe e saber se uma conta está ativa (a
// Posse de conta desativada vale como livre). Não autoriza: quem chama já
// autorizou o Ator.
type Directory struct{ users UserRepo }

func NewDirectory(users UserRepo) *Directory { return &Directory{users: users} }

// Lookup devolve as pessoas pedidas; id desconhecido fica de fora. A
// equipe é pequena (dezenas de pessoas): ler a lista inteira é mais
// simples que uma consulta por ids e custa o mesmo.
func (d *Directory) Lookup(ctx context.Context, ids []string) (map[string]Person, error) {
	out := map[string]Person{}
	if len(ids) == 0 {
		return out, nil
	}
	us, err := d.users.List(ctx)
	if err != nil {
		return nil, err
	}
	for _, u := range us {
		if slices.Contains(ids, u.ID) {
			out[u.ID] = Person{ID: u.ID, Name: u.Name, Active: u.Active()}
		}
	}
	return out, nil
}
