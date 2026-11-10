// Package noexit предоставляет анализатор, запрещающий прямой вызов os.Exit
// внутри функции main пакета main.
//
// Мотивация: вызов os.Exit в main немедленно завершает процесс, не выполняя
// отложенные вызовы defer. В результате не освобождаются ресурсы, не
// сбрасываются буферы и не закрываются сетевые соединения. Идиоматичная
// альтернатива — вынести тело main в helper-функцию (например, run), которая
// возвращает код ошибки/выхода, а уже после выполнения всех defer передать этот
// код в os.Exit или просто завершить main обычным возвратом.
package noexit

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

// Analyzer — экспортируемый экземпляр анализатора для подключения к
// multichecker.
var Analyzer = &analysis.Analyzer{
	Name: "noexit",
	Doc:  "reports direct calls to os.Exit in the main function of the main package",
	Run:  run,
}

// run обходит все файлы проверяемого пакета. Если пакет не является main,
// анализатор сразу завершается (os.Exit в других пакетах — например, в
// утилитах и библиотеках — не является предметом проверки). Внутри пакета main
// ищется объявление функции с именем main без получателя, и в её теле
// отыскиваются вызовы os.Exit.
func run(pass *analysis.Pass) (any, error) {
	if pass.Pkg.Name() != "main" {
		return nil, nil
	}

	for _, file := range pass.Files {
		// Сгенерированный код (например, сгенерированный драйвер `go test` с
		// собственной main, вызывающей os.Exit) — это вывод инструмента, а не
		// рукописный код автора, поэтому он не проверяется.
		if ast.IsGenerated(file) {
			continue
		}

		ast.Inspect(file, func(n ast.Node) bool {
			fn, ok := n.(*ast.FuncDecl)
			if !ok {
				return true
			}
			// Игнорируем методы (Recv != nil) и функции с другим именем.
			if fn.Recv != nil || fn.Name.Name != "main" {
				return true
			}

			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				if isOsExit(pass, call) {
					pass.Reportf(call.Pos(),
						"direct call to os.Exit in main is forbidden: it skips deferred calls; return an error or exit code from a helper function instead")
				}
				return true
			})
			return true
		})
	}

	return nil, nil
}

// isOsExit сообщает, является ли call вызовом os.Exit, где «os» — это именно
// пакет стандартной библиотеки os, а не произвольный идентификатор с именем os
// (локальная переменная или поле структуры).
func isOsExit(pass *analysis.Pass, call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Exit" {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	if !ok || id.Name != "os" {
		return false
	}

	// По возможности уточняем тип: «os» должен ссылаться на импортированный
	// пакет "os". Если типовой информации нет (синтаксически некорректный код),
	// полагаемся только на имя идентификатора.
	if pass.TypesInfo != nil {
		pkgName, ok := pass.TypesInfo.Uses[id].(*types.PkgName)
		if !ok {
			return false
		}
		return pkgName.Imported().Path() == "os"
	}
	return true
}
