# Mocking in kubectl-ai

## Gomock developer workflow

We use [gomock](https://github.com/uber-go/mock) to mock external dependencies. All mocks and generated files live under `internal/mocks/`.

- **Everyday commands**
  - Regenerate mocks after changing interfaces or adding new ones: `make generate`
  - Verify nothing is stale (locally or in CI): `make verify-mocks`
  - Run tests: `go test ./...`
- **Generator install** (if you don’t have it yet):  
  `go install go.uber.org/mock/mockgen@latest`
- **What `make generate` does**  
  Runs `go generate ./internal/mocks`. Note: `go generate` is **not** part of `go build/test`; commit generated mocks.
- **Add a new mock**
  1. Add a `go:generate` line in `internal/mocks/generate.go`, e.g.:  
     ```go
     //go:generate mockgen -destination=tools_mock.go -package=mocks      //  github.com/GoogleCloudPlatform/kubectl-ai/pkg/tools Tool
     ```
  2. Run `make generate` and import the mocks in tests:
     ```go
     ctrl := gomock.NewController(t)
     defer ctrl.Finish()

     llm := mocks.NewMockClient(ctrl) // example
     llm.EXPECT().NewChat(gomock.Any()).AnyTimes()
     ```

## When and when not to use gomock

**Use gomock for:**
- **External boundaries / side effects**: `gollm.Client`, `gollm.Chat`, `pkg/tools.Tool`, network/IO, anything slow or flaky.
- **Behavioral checks**: asserting specific calls/arguments or injecting failures/timeouts.

**Prefer fakes/in‑memory over mocks for:**
- **Stateful components with an in‑memory impl** (e.g., session/message store). Don’t mock storage if an in‑memory version exists.
- **Pure functions / simple value types**—call them directly.

**Good practices:**
- Keep expectations minimal—assert only what matters. Use `gomock.Any()` and `AnyTimes()`/`MinTimes(1)` where exact call counts don’t matter.
- Centralize `mockgen` directives in `internal/mocks/generate.go`.
- **If an interface changes**: run `make generate`, fix compile errors in tests (signatures/matchers), update/remove `go:generate` lines if package paths or names changed, and commit the regenerated mocks.

