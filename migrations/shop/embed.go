// Package shopmigrations встраивает SQL-миграции отдельной БД интернет-магазина
// (tea_shop) через go:embed. Магазин — самостоятельный bounded context со своей
// схемой, не пересекающейся с ресторанной (tea_platform).
package shopmigrations

import "embed"

// FS содержит все *.sql миграции магазина.
//
//go:embed *.sql
var FS embed.FS
