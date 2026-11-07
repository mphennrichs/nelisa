// Package migrations embute os arquivos SQL do goose no binário.
package migrations

import "embed"

// FS contém todas as migrations (*.sql) deste diretório.
//
//go:embed *.sql
var FS embed.FS
