// Package password guarda senhas com bcrypt. Só o hash; nunca a senha.
package password

import "golang.org/x/crypto/bcrypt"

// Bcrypt implementa a porta PasswordHasher dos módulos.
type Bcrypt struct{ cost int }

// NewBcrypt usa o custo padrão da biblioteca (10).
func NewBcrypt() Bcrypt { return Bcrypt{cost: bcrypt.DefaultCost} }

func (b Bcrypt) Hash(plain string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(plain), b.cost)
	return string(h), err
}

func (b Bcrypt) Verify(plain, hash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}
