package helper

import (
	"bytes"
	"testing"
	"text/template"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildInfoHelpers(t *testing.T) {
	tests := []struct {
		name    string
		version string
		date    string
		commit  string
		want    string
	}{
		{
			name:    "provided values",
			version: "v1.2.3",
			date:    "2026-11-08",
			commit:  "abc123",
			want:    "Build version: v1.2.3\nBuild date: 2026-11-08\nBuild commit: abc123\n",
		},
		{
			name: "empty values",
			want: "Build version: N/A\nBuild date: N/A\nBuild commit: N/A\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldVersion, oldDate, oldCommit := buildVersion, buildDate, buildCommit
			t.Cleanup(func() {
				buildVersion, buildDate, buildCommit = oldVersion, oldDate, oldCommit
			})

			buildVersion, buildDate, buildCommit = tt.version, tt.date, tt.commit
			var output bytes.Buffer
			PrintBuildInfo(&output)
			assert.Equal(t, tt.want, output.String())
			assert.Equal(t, valueOrNA(tt.version), BuildVersion())
			assert.Equal(t, map[string]string{
				"buildDate":   valueOrNA(tt.date),
				"buildCommit": valueOrNA(tt.commit),
			}, CobraAnnotations())
		})
	}
}

func TestCobraVersionTemplate(t *testing.T) {
	tmpl, err := template.New("version").Parse(CobraVersionTemplate)
	require.NoError(t, err)

	data := struct {
		Version     string
		Annotations map[string]string
	}{
		Version: "v1.2.3",
		Annotations: map[string]string{
			"buildDate":   "2026-11-08",
			"buildCommit": "abc123",
		},
	}

	var output bytes.Buffer
	require.NoError(t, tmpl.Execute(&output, data))
	assert.Equal(t, "Build version: v1.2.3\nBuild date: 2026-11-08\nBuild commit: abc123\n", output.String())
}
