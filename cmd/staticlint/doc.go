/*
Command staticlint is a multichecker — a static analysis driver that runs many
independent checks over Go source code from a single binary. It combines
analyzers from several sources:

 1. the standard analyzers of golang.org/x/tools/go/analysis/passes;
 2. the entire SA (correctness) class of staticcheck.io;
 3. the S (simplifications) class of staticcheck.io;
 4. a custom analyzer, noexit, that forbids direct calls to os.Exit in the main
    function of the main package.

# Running the multichecker

Build the binary:

	go build -o staticlint ./cmd/staticlint

Analyze every package of the current module (including tests):

	./staticlint ./...

or, without building an explicit binary:

	go run ./cmd/staticlint ./...

The tool accepts the same arguments as go vet: package patterns, directories or
files (for example, ./..., ./internal/..., ./cmd/...).

Available flags and subcommands:

	./staticlint help                 # list all analyzers and global flags
	./staticlint help <name>          # docs and flags of a specific analyzer
	./staticlint -test=false ./...    # skip test files
	./staticlint -SA1000=false ./...  # disable a specific check by name

When problems are found, the multichecker prints diagnostics in the form
"file:line:column: message" and exits with a non-zero code; when there are no
findings it exits silently with code 0.

# Analyzers

## Standard analyzers (golang.org/x/tools/go/analysis/passes)

Each analyzer below checks a separate class of common Go mistakes:

  - atomic — mistakes when using the sync/atomic package;
  - bools — mistakes involving boolean operators (e.g. redundant ! or comparisons);
  - buildtag — build tags are well-formed;
  - composite — unkeyed composite literals of types from other packages (they
    break when fields are added or reordered);
  - copylocks — locks (mutexes and the like) passed by value;
  - deepequalerrors — errors compared with reflect.DeepEqual instead of errors.Is;
  - defers — defer inside loops (accumulates deferred calls);
  - directive — malformed toolchain directives such as //go:...;
  - errorsas — the second argument of errors.As is valid (a pointer to error or
    to a type implementing error);
  - httpresponse — mistakes using http.Response (e.g. reading the body after Close);
  - ifaceassert — interface-to-interface type assertions without a check;
  - loopclosure — references to loop variables from nested functions that outlive
    the iteration;
  - lostcancel — a lost cancel function from context.WithCancel/WithTimeout;
  - nilfunc — useless comparisons of functions with nil;
  - nilness — redundant/impossible nil checks and possible nil dereferences;
  - printf — format string/argument mismatch in printf-like functions;
  - shift — shifts equal to or exceeding the width of an integer;
  - slog — mistakes when using structured logging from log/slog;
  - sortslice — incorrect less functions in sort.Slice;
  - stdmethods — wrong signatures of standard methods (MarshalJSON, Read, Write,
    Close, etc.);
  - stringintconv — string(int) conversions that yield a character, not a number;
  - structtag — malformed struct field tags;
  - testinggoroutine — calls to t.Fatal/t.FailNow inside a non-test goroutine;
  - tests — common mistakes in test code;
  - timeformat — incorrect reference layouts in time.Format;
  - unmarshal — json.Unmarshal passed a non-pointer/non-interface value;
  - unreachable — unreachable code;
  - unsafeptr — incorrect uintptr <-> unsafe.Pointer conversions;
  - unusedresult — unused results of functions such as fmt.Errorf, errors.New,
    strings.Replace;
  - unusedwrite — writes to struct fields that are never read.

## SA class of staticcheck.io (honnef.co/go/tools/staticcheck)

All analyzers of the SA class are enabled (names SA1000…SA9010). They report real
correctness bugs, grouped into categories:

  - SA1xxx — misuse of the standard library;
  - SA2xxx — concurrency bugs;
  - SA3xxx — issues in tests;
  - SA4xxx — ineffective code (dead code, unused values);
  - SA5xxx — correctness bugs;
  - SA6xxx — performance issues;
  - SA9xxx — dubious code.

See https://staticcheck.dev/docs/checks/ for the full description of every check.

## S class of staticcheck.io (honnef.co/go/tools/simple)

All analyzers of the S class are enabled (names S1000…S1040). They do not find
bugs; instead they suggest simpler, more idiomatic equivalents — for example,
replacing a single-case select with a plain channel send, or dropping a redundant
fmt.Sprintf.

## Custom analyzer: noexit

noexit forbids a direct call to os.Exit inside the main function of the main
package. Calling os.Exit terminates the process immediately without running
defers, so resources are not released and buffers are not flushed. The idiomatic
alternative is to return an error or exit code from a helper (run) function and
handle it in main. The analyzer skips generated code (for example, the generated
`go test` driver whose main calls os.Exit) and ignores os.Exit outside the main
function or in packages that are not main.
*/
package main
