// Package migrations встраивает SQL-файлы миграций в бинарник через go:embed,
// чтобы сервисы могли применять схему без внешних файлов.
package migrations

import "embed"

// FS содержит все *.sql миграции каталога.
//
//go:embed *.sql
var FS embed.FS
