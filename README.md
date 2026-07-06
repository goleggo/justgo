# JustGo CLI - Go Scaffolding & Code Generation Tool

JustGo is a lightweight, interactive command-line interface (CLI) tool designed to accelerate Go development. It structures projects using clean architecture patterns, integrates database connection pooling with automatic dependency injection (auto-wiring), auto-scaffolds Docker deployment contexts, sets up out-of-the-box observability libraries, and provides granular layer-by-layer domain code generation.

---

## Key Features

1. **Clean Architecture Layout Scaffolding**: Builds folder trees (`cmd/`, `internal/`, `pkg/`, `deployments/`, etc.) dynamically using a stack-based layout parser.
2. **Router Choice Support**: Choose between **Gin**, **Fiber v3**, or the **Standard Library** (`net/http`) ServeMux during configuration.
3. **Database Integration & Auto-Wiring (`sqlc`)**: Scaffold PostgreSQL (default), MySQL, or SQLite connections automatically, generating `sqlc.yaml`, DDL schemas, and query templates. Constructor functions (`Init`) and generated repositories are automatically wired with `*sql.DB` parameters when database scaffolding is active.
4. **Configuration Binder**: Configures `.env` loaders using `godotenv` to safely bind parameters (including database URL and observability credentials) to type-safe Go structs.
5. **Observability Stack (`goleggo/observer`)**: Integrate `github.com/goleggo/observer@v0.1.4` out of the box to set up slog structured logging, OpenTelemetry tracing, and middleware (such as Gin logger handlers) with minimal manual configuration.
6. **Graceful Shutdown**: Signal listeners (`SIGINT`/`SIGTERM`) shut down HTTP routing servers and database pools sequentially, waiting up to 5 seconds for active requests to finish.
7. **Bounded Code Generation**: Generate whole domain packages, individual architecture layers, or append specific route endpoints dynamically to files.
8. **Automated Mock Generation (`mockgen`)**: Generates mock structures under a dedicated `mocks/` folder for usecases and repositories automatically. Integrates native `//go:generate` directives and maps a `make mock` target in the `Makefile` to allow on-demand mock regeneration.

---

## Installation & Compilation

To build the `justgo` CLI executable, compile the source files locally:

```bash
cd justgo
go build -o justgo main.go
```

Place the compiled binary in your local environment path (like `~/bin` or `/usr/local/bin`) to run it globally.

---

## Usage Guide

### 1. Scaffold a New Project
Run the `new` command inside any directory:

```bash
./justgo new
```
This prompts you interactively for:
*   Project Name
*   Go Version (auto-detects system's installed Go version)
*   Router (Gin, Fiber v3, or Standard Library)
*   Database Scaffolding (PostgreSQL, MySQL, SQLite, or None)
*   Observability Stack (goleggo/observer integration toggle)
*   Custom dependencies to pre-install

Upon confirmation, it creates files, initializes the Go module, runs `go get`, and cleans dependencies.

### 2. Code Generation (`gen`)

#### Generate a Full Domain Module
Generates model, repository, usecase, handler, initialization, and routing configurations, and wires them into the main routing engine:
```bash
./justgo gen <domainName>

# Example:
./justgo gen billing
```
*Note: If database scaffolding is enabled, this automatically passes the `db` pool from `main.go` through `Init(db)` to the repository constructors.*

#### Generate a Single Layer Only
Generate individual clean architecture layers on-demand without writing the full package stack:
```bash
./justgo gen <layer> <domainName>

# Example (generates only model schema file):
./justgo gen model billing
```
*Supported layers: `model`, `repository`, `usecase`, `handler`, `init`, `routes`.*

#### Append a Single Endpoint Action
Append a new endpoint method to the handler and map its route mapping inside `routes.go` automatically:
```bash
./justgo gen handler <domainName> <actionName> [options]

# Example:
./justgo gen handler billing Create --method=POST --path=/api/v1/billing/create
```
*Options:*
*   `--method`: Specifies the HTTP method (`GET`, `POST`, `PUT`, `DELETE`). *Default: GET*
*   `--path`: Custom endpoint path. *Default: /<domainName>/<actionName>*

### 3. Regenerate Mocks
Mocks are automatically created during module generation. If you add or modify any interface definitions in the repository or usecase layers, you can regenerate the mock files by running:
```bash
make mock
```
This runs the native Go command `go generate ./...` in the background.

---

## Template Overrides

JustGo embeds standard boilerplate layouts. You can override these with your own custom designs by placing template files inside a `templates/` folder in the directory you run the command.

The resolution order is:
1.  `templates/<router>/<name>.tmpl`
2.  `templates/<name>.tmpl`
3.  Embedded fallback configurations

### Directory Layout for Overrides:
```text
templates/
├── gin/
│   ├── main.go.tmpl
│   ├── handler.tmpl
│   └── routes.tmpl
├── Makefile.tmpl
├── model.tmpl
├── repository.tmpl
├── usecase.tmpl
├── init.tmpl
├── env.tmpl
└── config.tmpl
```
You can use Go's `text/template` syntax inside these templates. The templates receive a configuration struct with the following boolean fields:
*   `.UseDB`: True if database scaffolding is enabled.
*   `.UseObs`: True if the observability stack is enabled.
