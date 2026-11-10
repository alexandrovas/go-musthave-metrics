package noexit_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/alexandrovas/go-musthave-metrics/cmd/staticlint/noexit"
)

func TestNoExit(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), noexit.Analyzer, "main", "other", "genmain")
}
