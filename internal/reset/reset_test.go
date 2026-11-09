package reset

import (
	"bytes"
	"go/ast"
	"go/token"
	"go/types"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/packages"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// --- вспомогательные конструкторы синтетических типов go/types ---

func testPackage() *types.Package {
	return types.NewPackage("example.com/test", "test")
}

// newNamedStruct создаёт именованную структуру с указанными полями и методами.
func newNamedStruct(name string, fields *types.Struct, methods ...*types.Func) *types.Named {
	pkg := testPackage()
	tn := types.NewTypeName(token.NoPos, pkg, name, nil)
	pkg.Scope().Insert(tn)
	return types.NewNamed(tn, fields, methods)
}

// newResetMethod создаёт метод Reset() с указателем на named в качестве ресивера.
func newResetMethod(named *types.Named) *types.Func {
	recv := types.NewVar(token.NoPos, testPackage(), "r", types.NewPointer(named))
	sig := types.NewSignatureType(recv, nil, nil, types.NewTuple(), types.NewTuple(), false)
	return types.NewFunc(token.NoPos, named.Obj().Pkg(), "Reset", sig)
}

// --- unit-тесты чистых функций ---

func TestHasResetDirective(t *testing.T) {
	mk := func(lines ...string) *ast.CommentGroup {
		cg := &ast.CommentGroup{}
		for _, l := range lines {
			cg.List = append(cg.List, &ast.Comment{Text: l})
		}
		return cg
	}

	tests := []struct {
		name string
		doc  *ast.CommentGroup
		want bool
	}{
		{"nil comment", nil, false},
		{"exact", mk("// generate:reset"), true},
		{"no space after slashes", mk("//generate:reset"), true},
		{"trailing spaces", mk("// generate:reset  "), true},
		{"block comment", mk("/* generate:reset */"), true},
		{"among other comments", mk("// Doc line.", "// generate:reset"), true},
		{"with extra text", mk("// generate:reset for tests"), false},
		{"unrelated comment", mk("// generate: reset"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, hasResetDirective(tt.doc))
		})
	}
}

func TestReceiverName(t *testing.T) {
	tests := []struct {
		typeName string
		want     string
	}{
		{"ResetableStruct", "r"},
		{"Foo", "f"},
		{"bar", "b"},
		{"", "r"},
		{"_Foo", "r"},
		{"Ωmega", "r"},
	}

	for _, tt := range tests {
		t.Run(tt.typeName, func(t *testing.T) {
			assert.Equal(t, tt.want, receiverName(tt.typeName))
		})
	}
}

func TestReceiverType(t *testing.T) {
	t.Run("plain", func(t *testing.T) {
		named := newNamedStruct("Foo", types.NewStruct(nil, nil))
		assert.Equal(t, "Foo", receiverType(named))
	})

	t.Run("generic", func(t *testing.T) {
		pkg := testPackage()
		tn := types.NewTypeName(token.NoPos, pkg, "Box", nil)
		named := types.NewNamed(tn, types.NewStruct(nil, nil), nil)

		tp := types.NewTypeParam(types.NewTypeName(token.NoPos, pkg, "T", nil), types.NewInterfaceType(nil, nil))
		named.SetTypeParams([]*types.TypeParam{tp})

		assert.Equal(t, "Box[T]", receiverType(named))
	})
}

func TestBasicZero(t *testing.T) {
	tests := []struct {
		kind   types.BasicKind
		want   string
		wantOK bool
	}{
		{types.Int, "0", true},
		{types.Uint8, "0", true},
		{types.Float64, "0", true},
		{types.Complex128, "0", true},
		{types.String, `""`, true},
		{types.Bool, "false", true},
	}

	for _, tt := range tests {
		t.Run(types.Typ[tt.kind].String(), func(t *testing.T) {
			got, ok := basicZero(types.Typ[tt.kind])
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFieldOps(t *testing.T) {
	intT := types.Typ[types.Int]
	stringT := types.Typ[types.String]

	withResetMarked := newNamedStruct("Marked", types.NewStruct(nil, nil))
	marked := map[*types.TypeName]bool{withResetMarked.Obj(): true}

	// Метод Reset() добавляется после создания типа: newResetMethod принимает
	// уже созданный named-тип в качестве ресивера.
	withResetMethod := newNamedStruct("MethodReset", types.NewStruct(nil, nil))
	withResetMethod.AddMethod(newResetMethod(withResetMethod))

	plain := newNamedStruct("Plain", types.NewStruct(nil, nil))

	tests := []struct {
		name   string
		t      types.Type
		marked map[*types.TypeName]bool
		want   []op
	}{
		{
			name: "int",
			t:    intT,
			want: []op{{Kind: kindAssign, Expr: "r.f", Zero: "0"}},
		},
		{
			name: "string",
			t:    stringT,
			want: []op{{Kind: kindAssign, Expr: "r.f", Zero: `""`}},
		},
		{
			name: "slice",
			t:    types.NewSlice(intT),
			want: []op{{Kind: kindSlice, Expr: "r.f", From: "r.f"}},
		},
		{
			name: "map",
			t:    types.NewMap(stringT, stringT),
			want: []op{{Kind: kindMap, Expr: "r.f"}},
		},
		{
			name: "interface",
			t:    types.NewInterfaceType(nil, nil),
			want: []op{{Kind: kindAssert, Expr: "r.f"}},
		},
		{
			name: "pointer to interface",
			t:    types.NewPointer(types.NewInterfaceType(nil, nil)),
			want: []op{
				{Kind: kindGuard, Expr: "r.f"},
				{Kind: kindAssert, Expr: "(*r.f)"},
				{Kind: kindEnd},
			},
		},
		{
			name: "pointer to string",
			t:    types.NewPointer(stringT),
			want: []op{
				{Kind: kindGuard, Expr: "r.f"},
				{Kind: kindAssign, Expr: "*r.f", Zero: `""`},
				{Kind: kindEnd},
			},
		},
		{
			name: "pointer to slice",
			t:    types.NewPointer(types.NewSlice(intT)),
			want: []op{
				{Kind: kindGuard, Expr: "r.f"},
				{Kind: kindSlice, Expr: "*r.f", From: "(*r.f)"},
				{Kind: kindEnd},
			},
		},
		{
			name:   "marked struct calls Reset",
			t:      withResetMarked,
			marked: marked,
			want:   []op{{Kind: kindReset, Expr: "r.f"}},
		},
		{
			name: "struct with own Reset method",
			t:    withResetMethod,
			want: []op{{Kind: kindReset, Expr: "r.f"}},
		},
		{
			name: "struct without Reset is skipped",
			t:    plain,
			want: nil,
		},
		{
			name:   "pointer to marked struct",
			t:      types.NewPointer(withResetMarked),
			marked: marked,
			want: []op{
				{Kind: kindGuard, Expr: "r.f"},
				{Kind: kindReset, Expr: "r.f"},
				{Kind: kindEnd},
			},
		},
		{
			name: "chan is skipped",
			t:    types.NewChan(types.SendRecv, intT),
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fieldOps("r.f", tt.t, tt.marked)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestHasReset(t *testing.T) {
	markedStruct := newNamedStruct("Marked", types.NewStruct(nil, nil))
	marked := map[*types.TypeName]bool{markedStruct.Obj(): true}

	methodStruct := newNamedStruct("MethodReset", types.NewStruct(nil, nil))
	methodStruct.AddMethod(newResetMethod(methodStruct))
	plainStruct := newNamedStruct("Plain", types.NewStruct(nil, nil))

	tests := []struct {
		name   string
		t      types.Type
		marked map[*types.TypeName]bool
		want   bool
	}{
		{"marked type", markedStruct, marked, true},
		{"type with Reset method", methodStruct, nil, true},
		{"plain type", plainStruct, nil, false},
		{"basic type", types.Typ[types.Int], nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, hasReset(tt.t, tt.marked))
		})
	}
}

func TestRender(t *testing.T) {
	data := fileData{
		Package: "sample",
		Structs: []structData{
			{
				Recv: "s",
				Type: "S",
				Ops: []op{
					{Kind: kindAssign, Expr: "s.i", Zero: "0"},
					{Kind: kindSlice, Expr: "s.s", From: "s.s"},
					{Kind: kindMap, Expr: "s.m"},
				},
			},
		},
	}

	want := `// Code generated by reset; DO NOT EDIT.

package sample

func (s *S) Reset() {
	if s == nil {
		return
	}

	s.i = 0
	s.s = s.s[:0]
	clear(s.m)
}
`

	got, err := render(data)
	require.NoError(t, err)
	assert.Equal(t, want, string(got))
}

func TestWriteIfChanged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reset.gen.go")

	// Создание нового файла с правами по умолчанию.
	changed, err := writeIfChanged(path, []byte("v1"))
	require.NoError(t, err)
	assert.True(t, changed)
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "v1", string(got))
	assert.Equal(t, os.FileMode(0o644), mustPerm(t, path))

	// Тот же контент — записи нет.
	changed, err = writeIfChanged(path, []byte("v1"))
	require.NoError(t, err)
	assert.False(t, changed)

	// Новый контент — запись с сохранением исходных прав файла.
	require.NoError(t, os.Chmod(path, 0o640))
	changed, err = writeIfChanged(path, []byte("v2"))
	require.NoError(t, err)
	assert.True(t, changed)
	got, err = os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "v2", string(got))
	assert.Equal(t, os.FileMode(0o640), mustPerm(t, path))
}

func mustPerm(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	require.NoError(t, err)
	return info.Mode().Perm()
}

func TestIsBasePackage(t *testing.T) {
	tests := []struct {
		name string
		pkg  *packages.Package
		want bool
	}{
		{"base package", &packages.Package{ID: "example.com/p"}, true},
		{"test variant", &packages.Package{ID: "example.com/p [example.com/p.test]"}, false},
		{"ForTest set", &packages.Package{ID: "example.com/p", ForTest: "example.com/p"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isBasePackage(tt.pkg))
		})
	}
}

func TestNormalizePatterns(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "sub"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file.go"), []byte("package p\n"), 0o644))

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"bare directory gets ./ prefix", "sub", "./sub"},
		{"explicit relative pattern unchanged", "./sub", "./sub"},
		{"absolute path unchanged", "/abs/path", "/abs/path"},
		{"file path unchanged", "file.go", "file.go"},
		{"wildcard pattern unchanged", "sub/...", "sub/..."},
		{"dot unchanged", ".", "."},
		{"package name unchanged (not a dir)", "fmt", "fmt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizePatterns(dir, []string{tt.in})
			assert.Equal(t, []string{tt.want}, got)
		})
	}
}

// --- интеграционные тесты на временном модуле ---

// writeTestModule создаёт временный модуль с двумя помеченными структурами
// и возвращает его директорию и паттерн для загрузки.
func writeTestModule(t *testing.T) (dir, pattern string) {
	t.Helper()
	dir = t.TempDir()
	pattern = filepath.Join(dir, "...")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte(
		"module example.com/resettest\n\ngo 1.26\n"), 0o644))

	sampleDir := filepath.Join(dir, "sample")
	require.NoError(t, os.MkdirAll(sampleDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(sampleDir, "sample.go"), []byte(
		`package sample

// generate:reset
type Inner struct {
	x int
}

// generate:reset
type Box[T any] struct {
	value [0]T
}

type Dummy interface {
	Do()
}

// generate:reset
type S struct {
	i     int
	str   string
	strP  *string
	s     []int
	m     map[string]string
	child *Inner
	d     Dummy
}

func useResetBeforeGeneration() {
	(&S{}).Reset()
	(&Box[int]{}).Reset()
}
`), 0o644))

	return dir, pattern
}

const testModuleGolden = `// Code generated by reset; DO NOT EDIT.

package sample

func (b *Box[T]) Reset() {
	if b == nil {
		return
	}

}

func (i *Inner) Reset() {
	if i == nil {
		return
	}

	i.x = 0
}

func (s *S) Reset() {
	if s == nil {
		return
	}

	s.i = 0
	s.str = ""
	if s.strP != nil {
		*s.strP = ""
	}
	s.s = s.s[:0]
	clear(s.m)
	if s.child != nil {
		s.child.Reset()
	}
	if resetter, ok := s.d.(interface{ Reset() }); ok && s.d != nil {
		resetter.Reset()
	}
}
`

func TestRun(t *testing.T) {
	dir, pattern := writeTestModule(t)

	require.NoError(t, newGenerator(discardLogger()).run(dir, []string{pattern}))

	out := filepath.Join(dir, "sample", "reset.gen.go")
	got, err := os.ReadFile(out)
	require.NoError(t, err)
	assert.Equal(t, testModuleGolden, string(got))

	// Повторный запуск не меняет файл.
	require.NoError(t, newGenerator(discardLogger()).run(dir, []string{pattern}))
	got2, err := os.ReadFile(out)
	require.NoError(t, err)
	assert.Equal(t, testModuleGolden, string(got2))
}

func TestRun_GeneratedCodeCompiles(t *testing.T) {
	if testing.Short() {
		t.Skip("compilation check skipped in short mode")
	}

	dir, pattern := writeTestModule(t)

	require.NoError(t, newGenerator(discardLogger()).run(dir, []string{pattern}))

	cmd := exec.Command("go", "build", "./...")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "generated code does not compile:\n%s", out)
}

func TestRun_WriteError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}

	dir, pattern := writeTestModule(t)

	// Директория пакета только для чтения — запись файла невозможна.
	sampleDir := filepath.Join(dir, "sample")
	require.NoError(t, os.Chmod(sampleDir, 0o555))
	t.Cleanup(func() { _ = os.Chmod(sampleDir, 0o755) })

	err := newGenerator(discardLogger()).run(dir, []string{pattern})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "write")
}

func TestRun_FileAndDirectoryArguments(t *testing.T) {
	dir, _ := writeTestModule(t)
	out := filepath.Join(dir, "sample", "reset.gen.go")

	t.Run("single .go file", func(t *testing.T) {
		require.NoError(t, newGenerator(discardLogger()).run(dir, []string{"sample/sample.go"}))
		got, err := os.ReadFile(out)
		require.NoError(t, err)
		assert.Equal(t, testModuleGolden, string(got))
		require.NoError(t, os.Remove(out))
	})

	t.Run("bare directory", func(t *testing.T) {
		require.NoError(t, newGenerator(discardLogger()).run(dir, []string{"sample"}))
		got, err := os.ReadFile(out)
		require.NoError(t, err)
		assert.Equal(t, testModuleGolden, string(got))
	})
}

func TestRun_OutsideModule(t *testing.T) {
	// Каталог без go.mod — ни один пакет не загрузится, это ошибка запуска.
	err := newGenerator(nil).run(t.TempDir(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no packages loaded")
}

func TestRun_NoMarkedStructs(t *testing.T) {
	// Модуль без директивы // generate:reset — генерировать нечего, ошибка не
	// возвращается, но предупреждение попадает в лог.
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte(
		"module example.com/empty\n\ngo 1.26\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "plain.go"), []byte(
		"package plain\n\ntype S struct {\n\ti int\n}\n"), 0o644))

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	require.NoError(t, newGenerator(logger).run(dir, []string{"./..."}))
	assert.Contains(t, buf.String(), "no structs marked with // "+ResetDirective+" found")
}
