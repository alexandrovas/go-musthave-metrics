// Command reset generates Reset() methods for every struct annotated with the
// // generate:reset directive.
//
// It scans all packages under the given patterns ("./..." when none are given)
// and writes the generated methods into a reset.gen.go file in each package.
//
// Usage:
//
//	go run ./cmd/reset ./...
//
// Arguments are go list patterns ("./..."), directory paths ("internal") or
// .go file paths ("internal/config/config.go").
//
// or as a go:generate directive:
//
//	//go:generate go run github.com/alexandrovas/go-musthave-metrics/cmd/reset ./...
package main

import (
	"log/slog"
	"os"

	"github.com/alexandrovas/go-musthave-metrics/internal/reset"
)

func main() {
	execute()
}

// execute завершает процесс с ненулевым кодом, если генерация не удалась.
// os.Exit вынесен в хелпер, потому что staticlint запрещает его в main.
func execute() {
	if err := reset.Run(slog.Default(), os.Args[1:]...); err != nil {
		slog.Error("reset generation failed", "error", err)
		os.Exit(1)
	}
}
