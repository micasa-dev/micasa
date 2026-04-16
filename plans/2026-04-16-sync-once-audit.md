<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# sync.Once → sync.OnceValue/OnceValues/OnceFunc Audit Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Finish the #938 audit by replacing the two remaining `sync.Once`
call sites (both in `_test.go` files) with the Go 1.21+ one-shot wrappers,
so the "Do + pkg var + return var" boilerplate is retired project-wide and
`rg 'sync\.Once\b' --type go` comes back empty.

**Architecture:** The pattern in the issue example was `var + sync.Once + function`
returning a package-level pointer. Replacing it with a function-typed var initialised
by `sync.OnceValue(fn)` collapses three declarations into one and lets the return
flow directly out of the closure. Two call sites remain, both in test code: one
matches the pattern with an additional error return (→ `sync.OnceValues` +
`atomic.Pointer` for the `TestMain` cleanup signal) and one is a side-effect-only
driver registration (→ `sync.OnceFunc`). `internal/extract/tools.go` — the
production target called out in the issue and the motivation for nilaway
compatibility — was already migrated by PR #939; nilaway excludes test files
(`-exclude-test-files` in `.github/workflows/lint.yml`), so the remaining two
migrations are audit-completion, not nilaway-driven.

**Tech Stack:** Go 1.26, standard library `sync` and `sync/atomic`, `testify` for
assertions. No new dependencies.

---

## File Structure

Three Go files are modified. Each change is local; no cross-file refactor.

- `cmd/micasa/main_test.go` — replace three package-level test-binary vars with a
  `sync.OnceValues` closure and an `atomic.Pointer[string]` so `TestMain` can still
  decide whether to clean the temp dir.
- `internal/data/sqlite/sqlite_test.go` — hoist `customDriverName` const to package
  scope and convert the one-shot driver registration to `sync.OnceFunc`.
- `internal/extract/tools_test.go` — update a stale comment referencing `sync.Once`
  caching to match the now-migrated implementation (`sync.OnceValue`).

No new files. No tests to create: existing tests already exercise each migrated
path (`TestVersion_DevShowsCommitHash` drives `getTestBin`; `TestDialector` drives
`registerCustomDriver`; the `OCRAvailable` / `HasX` smoke tests drive
`DefaultOCRTools`).

---

### Task 1: Migrate `cmd/micasa/main_test.go` to `sync.OnceValues`

**Why:** Current code keeps three package-level vars (`testBin`, `testBinOnce`,
`errTestBin`) for what is semantically one lazily-built artefact plus its error.
`sync.OnceValues` collapses the closure-to-var dance; `atomic.Pointer[string]`
carries the success signal to `TestMain` so cleanup still only runs when the
build succeeded (matching the original `if testBin != ""` guard).

**Files:**
- Modify: `cmd/micasa/main_test.go` — add `sync/atomic` to the stdlib import
  block; rewrite the `TestMain` body to read from `testBinPath` instead of the
  removed `testBin` var; replace the `var (testBin, testBinOnce, errTestBin)`
  block and the `getTestBin` function with the `sync.OnceValues` form.

- [ ] **Step 1: Verify baseline tests pass**

Run: `go test -run TestVersion -count=1 ./cmd/micasa/`
Expected: PASS (exercises the existing `getTestBin` via
`TestVersion_DevShowsCommitHash`).

- [ ] **Step 2: Update imports**

Edit `cmd/micasa/main_test.go`:

Replace
```go
	"sync"
	"testing"
```
with
```go
	"sync"
	"sync/atomic"
	"testing"
```

- [ ] **Step 3: Replace package vars and `getTestBin` body**

Replace the `// testBin is a lazily-built...` comment, the `var (testBin,
testBinOnce, errTestBin)` block, and the `getTestBin` function with:

```go
// buildTestBin lazily builds the micasa CLI binary for the few tests that
// need subprocess isolation (env vars, VCS info). sync.OnceValues ensures
// the `go build` runs at most once across the whole test binary; the
// (path, error) pair is cached and returned to every caller.
var buildTestBin = sync.OnceValues(func() (string, error) {
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	dir, err := os.MkdirTemp("", "micasa-test-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}
	bin := filepath.Join(dir, "micasa"+ext)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "build", "-o", bin, ".")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		_ = os.RemoveAll(dir)
		return "", fmt.Errorf("build: %w\n%s", err, out)
	}
	testBinPath.Store(&bin)
	return bin, nil
})

// testBinPath records the built binary path so TestMain can remove the
// enclosing temp dir without re-triggering the build. Nil until a test
// successfully calls getTestBin.
var testBinPath atomic.Pointer[string]

func getTestBin(t *testing.T) string {
	t.Helper()
	bin, err := buildTestBin()
	require.NoError(t, err, "building test binary")
	return bin
}
```

- [ ] **Step 4: Update `TestMain` to read `testBinPath`**

Replace the existing `TestMain` body (the `if testBin != ""` cleanup guard)
with:

```go
func TestMain(m *testing.M) {
	code := m.Run()
	if bin := testBinPath.Load(); bin != nil {
		_ = os.RemoveAll(filepath.Dir(*bin))
	}
	os.Exit(code)
}
```

- [ ] **Step 5: Rerun the CLI tests**

Run: `go test -run TestVersion -count=1 ./cmd/micasa/`
Expected: PASS. The `_DevShowsCommitHash` subtest must still build the binary
successfully and report the commit hash.

- [ ] **Step 6: Rerun the full cmd/micasa package**

Run: `go test -shuffle=on ./cmd/micasa/`
Expected: PASS across all tests. Confirms no compile errors and no collateral
breakage (`TestMain` cleanup path in particular).

- [ ] **Step 7: Commit with `/commit`** (scope: `cli`; refactor of the
  test-binary builder).

---

### Task 2: Migrate the sqlite dialector test to `sync.OnceFunc`

**Why:** `registerCustomDriver` runs a side effect (`sql.Register`) exactly once.
No var is set and no value is returned, so the pattern does not match the literal
"set var, return var" wording of the issue — but `sync.OnceFunc` is the Go 1.21
equivalent for side-effect-only initialisers, and leaving one lone `sync.Once`
behind defeats the point of the audit. The closure needs `customDriverName`, so
hoist that constant to package scope (it is already used twice inside
`TestDialector`, so making it top-level is cheap).

**Files:**
- Modify: `internal/data/sqlite/sqlite_test.go` — add a package-level
  `customDriverName` const; replace the `var registerCustomDriver sync.Once`
  declaration with a `sync.OnceFunc` initializer that closes over the const;
  remove the redundant `const customDriverName` and `sql.Register` call from
  inside `TestDialector` and change the `.Do(func() { ... })` to a plain
  `registerCustomDriver()`.

- [ ] **Step 1: Verify baseline tests pass**

Run: `go test -run TestDialector -count=1 ./internal/data/sqlite/`
Expected: PASS.

- [ ] **Step 2: Replace package-level `testSeq` / `registerCustomDriver` block**

Find the existing block between the imports and `TestDialector`:

```go
var testSeq atomic.Uint64

// testDSN returns a unique shared in-memory DSN per test invocation to avoid
// cross-test lock contention on the same shared cache database.
func testDSN(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("file:%s_%d?mode=memory&cache=shared", t.Name(), testSeq.Add(1))
}

var registerCustomDriver sync.Once
```

Replace with:

```go
var testSeq atomic.Uint64

// testDSN returns a unique shared in-memory DSN per test invocation to avoid
// cross-test lock contention on the same shared cache database.
func testDSN(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("file:%s_%d?mode=memory&cache=shared", t.Name(), testSeq.Add(1))
}

const customDriverName = "test_custom_driver"

// registerCustomDriver registers a second modernc.org/sqlite driver under an
// alternate name so TestDialector can verify the DriverName override path.
// sync.OnceFunc guarantees sql.Register runs at most once across the test
// binary (a second registration would panic).
var registerCustomDriver = sync.OnceFunc(func() {
	sql.Register(customDriverName, &modernsqlite.Driver{})
})
```

- [ ] **Step 3: Update the call site inside `TestDialector`**

Find the opening lines of `TestDialector`:

```go
func TestDialector(t *testing.T) {
	const customDriverName = "test_custom_driver"

	registerCustomDriver.Do(func() {
		sql.Register(customDriverName, &modernsqlite.Driver{})
	})
```

Replace with:

```go
func TestDialector(t *testing.T) {
	registerCustomDriver()
```

(The const and the `sql.Register` call move to package scope via Step 2; the
remaining table entry `DriverName: customDriverName,` resolves against the
package-level const unchanged.)

- [ ] **Step 4: Rerun the full sqlite package**

Run: `go test -shuffle=on ./internal/data/sqlite/`
Expected: PASS for `TestDialector` (including the `custom_driver` subtest
that depends on `registerCustomDriver`) and every other test in the package.

- [ ] **Step 5: Commit with `/commit`** (scope: `data`; refactor of the
  sqlite driver-registration once).

---

### Task 3: Refresh stale `sync.Once` comment in `internal/extract/tools_test.go`

**Why:** The smoke-test comment on `TestOCRAvailable` still says "consistent
results across calls (sync.Once caching)." The implementation it refers to is
already `sync.OnceValue` (migrated in PR #939). Keep code comments truthful.

**Files:**
- Modify: `internal/extract/tools_test.go` — update the single `sync.Once`
  reference in the `TestOCRAvailable` doc comment.

- [ ] **Step 1: Edit the comment**

Replace
```go
	// Smoke test: just verify the functions don't panic and return
	// consistent results across calls (sync.Once caching).
```
with
```go
	// Smoke test: just verify the functions don't panic and return
	// consistent results across calls (sync.OnceValue caching).
```

- [ ] **Step 2: Confirm the test still builds**

Run: `go test -run TestOCRAvailable -count=1 ./internal/extract/`
Expected: PASS.

- [ ] **Step 3: Commit with `/commit`** (scope: `extract`; doc-comment
  realignment).

---

### Task 4: Full verification pipeline

**Why:** The three migrations are structural but touch two test files and one
test comment. Run the full project gate before opening a PR.

- [ ] **Step 1: Confirm no `sync.Once` references remain in Go code**

Run: `rg --type go 'sync\.Once\b' || echo CLEAN`
Expected: `CLEAN` (no matches). `sync.OnceValue`, `sync.OnceValues`, `sync.OnceFunc`
are fine — the bare `sync.Once` type is what should be gone.

- [ ] **Step 2: Build**

Run: `go build ./...`
Expected: exits 0, no output.

- [ ] **Step 3: Full test suite with race detector**

Run: `go test -race -shuffle=on ./...`
Expected: `ok` for every package, no FAIL. Matches the Linux CI command
(`.github/workflows/ci.yml:176`). Runs in under a few minutes on a dev
machine. Since the migrated helpers live in `_test.go` files (not reported
by `go test -coverprofile`), a green test run is the primary correctness
signal — the migrated helpers are only reachable via the tests that already
exercise them (`TestVersion_DevShowsCommitHash`, `TestDialector`,
`TestOCRAvailable`).

- [ ] **Step 4: Lint**

Run: `golangci-lint run`
Expected: exits 0, no findings.

---

### Task 5: Branch wrap-up and PR

**Why:** Ship the audit. One PR, three commits (one per migrated file), closes
the issue.

- [ ] **Step 1: Confirm commits**

Run: `git log --oneline upstream/main..HEAD`
Expected: the plan commit(s) followed by three implementation commits
(CLI test-bin, sqlite driver, extract comment) in that order.

- [ ] **Step 2: Push the branch**

Run: `git push -u origin refactor/issue-938`
Expected: branch published on `origin`.

- [ ] **Step 3: Open the PR with `/create-pr`**

Label the PR `refactor`. The body should map the three commits back to
the issue (one bullet per file: `main_test.go`, `sqlite_test.go`,
`tools_test.go`), note that `internal/extract/tools.go` was already
handled by PR #939, and include `Closes #938`.

- [ ] **Step 4: Comment on the issue**

Post a short `gh issue comment 938 --repo micasa-dev/micasa` note linking
to the PR URL and summarising which call sites moved to which wrapper.

---

## Self-Review

**Spec coverage:**
- Issue requires auditing every `sync.Once` → ✅ Tasks 1-3 cover the two remaining
  Go files; Task 4 Step 1 verifies no `sync.Once` references remain.
- Issue requires migrating where pattern matches → ✅ Task 1 uses `sync.OnceValues`
  (two-return variant, explicit per issue text); Task 2 uses `sync.OnceFunc`
  (documented as a deliberate extension for the side-effect pattern so no
  stragglers remain).
- Primary target (`internal/extract/tools.go`) — already landed via PR #939,
  called out in the plan header and the PR body.

**Placeholder scan:** No "TBD", "implement later", or bare "add error handling"
directives. Every code step shows the full edit. Every run step shows the exact
command and the expected outcome.

**Type consistency:**
- `buildTestBin` (Task 1) is `func() (string, error)`; call sites use
  `bin, err := buildTestBin()` — matches.
- `testBinPath` (Task 1) is `atomic.Pointer[string]`; producer uses `.Store(&bin)`,
  consumer uses `.Load()` — matches.
- `registerCustomDriver` (Task 2) is `func()` (return of `sync.OnceFunc`); call
  site uses `registerCustomDriver()` — matches.
- `customDriverName` (Task 2) is a `const string` at package scope; referenced
  by both `sql.Register` (inside the OnceFunc closure) and the `DriverName:
  customDriverName` table entry (inside `TestDialector`) — both references
  resolve against the package-level const after the hoist.
