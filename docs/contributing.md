# Contributing

Thank you for your interest in contributing to MIE. This guide covers how to set up your development environment, run tests, and submit changes.

## Development setup

### Prerequisites

- **Go 1.24+** ([install](https://go.dev/dl/))
- **C compiler** (gcc or clang, for CozoDB CGO bindings)
- **make**

### Clone and build

```bash
git clone https://github.com/kraklabs/mie.git
cd mie
make deps    # Downloads CozoDB static library (~30 MB)
make build   # Builds bin/mie
```

The `make deps` target downloads the correct CozoDB C library for your platform (macOS arm64/x86_64 or Linux arm64/x86_64) and places it in `lib/`.

### Build details

MIE uses CGO to link against CozoDB's C library. The build requires:

- Build tag: `cozodb`
- CGO_ENABLED: `1`
- CGO_LDFLAGS: `-L$(pwd)/lib -lcozo_c -lstdc++ -lm` (plus `-framework Security` on macOS)

These are handled automatically by the Makefile.

### Install development tools

```bash
make tools
```

This installs:

- `golangci-lint` -- Linter
- `goimports` -- Import formatting

## Running tests

### Full test suite

```bash
make test
```

Runs all tests with race detection and generates a coverage report at `coverage.out`.

### Short tests (no CozoDB required)

```bash
make test-short
```

Runs tests with the `-short` flag, skipping integration tests that need a CozoDB database.

### View coverage

```bash
make test-coverage
```

Opens the coverage report in your browser.

## Code style

### Linting

```bash
make lint
```

MIE uses `golangci-lint` with the project's default configuration. All code must pass linting before merge.

### Formatting

```bash
make fmt
```

Runs `go fmt` and `goimports` on all Go files.

To check formatting without modifying files (useful in CI):

```bash
make fmt-check
```

### Copyright headers

All Go source files must include the copyright header:

```go
// Copyright (C) 2025-2026 Kraklabs. All rights reserved.
// Use of this source code is governed by the AGPL-3.0
// license that can be found in the LICENSE file.
```

### Build tags

Files that import `pkg/storage` or `pkg/cozodb` must include the build tag:

```go
//go:build cozodb
```

This ensures the project can be used as a library without requiring the CozoDB C dependency.

## Project structure

```
mie/
  cmd/mie/          CLI and MCP server entrypoint
  pkg/
    memory/         Core domain: schema, writer, reader, conflicts, embedding, client
    tools/          MCP tool definitions and Querier interface
    storage/        CozoDB backend wrapper
    cozodb/         Low-level CozoDB CGO bindings
  lib/              CozoDB static library (downloaded by make deps)
  docs/             Documentation
```

## Makefile targets

| Target | Description |
|--------|-------------|
| `make build` | Build the `mie` binary to `bin/mie`. |
| `make install` | Install `mie` to `~/go/bin` (override with `INSTALL_DIR=path`). |
| `make deps` | Download CozoDB static library. |
| `make test` | Run all tests with race detection and coverage. |
| `make test-short` | Run tests without integration tests. |
| `make test-coverage` | Open coverage report in browser. |
| `make lint` | Run `golangci-lint`. |
| `make fmt` | Format all Go files. |
| `make fmt-check` | Check formatting (for CI). |
| `make tools` | Install development tools. |
| `make clean` | Remove build artifacts. |
| `make docker-build` | Build Docker image. |
| `make docker-push` | Push Docker image to registry. |

## Pull request process

1. Fork the repository and create a feature branch.
2. Make your changes. Follow the code style guidelines above.
3. Add tests for new functionality.
4. Run `make lint test` and ensure everything passes.
5. Submit a pull request with a clear description of the change.

### PR checklist

- [ ] Code compiles with `make build`
- [ ] All tests pass with `make test`
- [ ] Linting passes with `make lint`
- [ ] New files include the copyright header
- [ ] Files importing storage/cozodb have the `//go:build cozodb` tag
- [ ] New MCP tool parameters are documented

## License

MIE is licensed under the [GNU Affero General Public License v3.0 (AGPL-3.0)](https://www.gnu.org/licenses/agpl-3.0.html).

By contributing, you agree that your contributions will be licensed under the same license. A Contributor License Agreement (CLA) may be required for significant contributions -- the maintainers will reach out if needed.