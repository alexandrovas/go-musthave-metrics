package helper

import (
	"fmt"
	"io"
)

// Эти переменные задаются линковщиком через -ldflags -X при сборке бинаря.
var (
	buildVersion string
	buildDate    string
	buildCommit  string
)

// PrintBuildInfo выводит сведения о сборке.
func PrintBuildInfo(w io.Writer) {
	fmt.Fprintf(w, "Build version: %s\nBuild date: %s\nBuild commit: %s\n",
		valueOrNA(buildVersion), valueOrNA(buildDate), valueOrNA(buildCommit))
}

// valueOrNA заменяет пустое значение на N/A.
func valueOrNA(v string) string {
	if v == "" {
		return "N/A"
	}
	return v
}

// BuildVersion возвращает версию для поля Version команды Cobra.
func BuildVersion() string {
	return valueOrNA(buildVersion)
}

// CobraVersionTemplate задаёт формат вывода команды с флагом --version.
const CobraVersionTemplate = `Build version: {{.Version}}
Build date: {{index .Annotations "buildDate"}}
Build commit: {{index .Annotations "buildCommit"}}
`

// CobraAnnotations передаёт дату и commit в шаблон --version Cobra.
func CobraAnnotations() map[string]string {
	return map[string]string{
		"buildDate":   valueOrNA(buildDate),
		"buildCommit": valueOrNA(buildCommit),
	}
}
