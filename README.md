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
9. **Modular Monolith (Hexagonal) Architecture**: An alternative project blueprint, selectable during `justgo new`, that splits the app into isolated bounded-context modules (`modules/<name>/`) communicating through Ports & Adapters, with an experimental command to extract a module into its own standalone microservice. See [Modular Monolith (Hexagonal) Architecture](#modular-monolith-hexagonal-architecture) below.

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
*   **Architecture Style**: Standard Clean Architecture (default) or **Modular Monolith (Hexagonal)** — see [Modular Monolith (Hexagonal) Architecture](#modular-monolith-hexagonal-architecture)
*   Database Scaffolding (PostgreSQL, MySQL, SQLite, or None)
*   Observability Stack (goleggo/observer integration toggle)
*   Cross-Module Communication (Hexagonal only): Direct Synchronous Calls, In-Memory Dispatcher, or Watermill (RabbitMQ/Kafka)
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

### 3. Generate Agent/Harness Instructions (`agents`)

Generate a short instructional doc that teaches an AI coding agent how to work with this justgo-scaffolded project — which commands to run to add domains/layers/endpoints, and which generated files/markers not to hand-edit (`// [justgo:imports]`, `// [justgo:wire]`, `// [justgo:routes]`, and the `mocks/` folder).

```bash
./justgo agents
```

You'll be prompted to pick which format(s) to write, since different agents/harnesses read different files:

```
1. AGENTS.md (universal: Claude Code, Codex, Cursor, VS Code, ...)
2. Claude Code Skill (.claude/skills/justgo-workflow/SKILL.md)
3. Kiro steering (.kiro/steering/justgo-workflow.md)
4. All of the above
```

All three share the same content (tailored to the project's router/DB/observability config from `.justgo.json`) and only differ in file location and frontmatter. The command is safe to rerun any time — e.g. after enabling database scaffolding — it just overwrites the selected file(s).

### 4. Regenerate Mocks
Mocks are automatically created during module generation. If you add or modify any interface definitions in the repository or usecase layers, you can regenerate the mock files by running:
```bash
make mock
```
This runs the native Go command `go generate ./...` in the background.

---

## Modular Monolith (Hexagonal) Architecture

Projects scaffolded with `justgo new` → **Architecture Style: Modular Monolith (Hexagonal)** organize business logic into isolated bounded-context **modules** under `modules/<name>/`, each communicating through Ports & Adapters instead of directly reaching into each other's internals. This is based on the [Combining Modular Monolith and Hexagonal](https://notes.softwarearchitect.id/p/combining-modular-monolith-and-hexagonal) / [Developing Modular Monolith and Hexagonal](https://notes.softwarearchitect.id/p/developing-modular-monolith-and-hexagonal) architecture pattern.

### Module Anatomy
```text
modules/billing/
├── billing.go                      # public API: domain entity + inbound Service port (interface)
├── internal/
│   ├── ports/repository.go         # outbound port: Repository interface (private to this module)
│   ├── service/billing_service.go  # Service implementation (business logic)
│   └── adapter/
│       ├── http/billing_handler.go       # inbound adapter (HTTP controller)
│       └── repository/billing_repository.go # outbound adapter (DB access + ToDomain/FromDomain mapping)
├── factory/factory.go              # wires repository -> service -> handler (this module's only DI code)
├── module/module.go                # thin wrapper exposing New(...) + RegisterRoutes(...) for cmd/ composition
└── mocks/mock_repository.go        # mockgen output for the Repository port
```

Go's compiler enforces the boundary: only code rooted at `modules/billing/...` can import `modules/billing/internal/...`. Other modules — and `cmd/`— may only ever depend on the public API file (`billing.go`) and `module/module.go`. If one module needs to call another synchronously, define a small local interface covering just the methods you need in the calling module rather than importing the other module's public `Service` wholesale (see the comment in `factory/factory.go`) — this mirrors how services would call each other over the network if later split apart.

Each module also owns its own DB schema/queries under `db/modules/<name>/`, wired as an isolated `sql:` entry in the project's `sqlc.yaml` — no module should read or write another module's tables directly.

### Generate a Module
```bash
./justgo gen module <name>

# Example:
./justgo gen module billing
```
This scaffolds the full module tree above, wires it into `cmd/<project>/main.go` via the `// [justgo:imports]` / `// [justgo:wire]` markers (calling `<name>.New(db, msgBus).RegisterRoutes(...)`), adds its `sql:` entry to `sqlc.yaml` if database scaffolding is enabled, runs `go mod tidy`, and regenerates its repository mock. Only available in projects scaffolded with the Hexagonal architecture (`.justgo.json`'s `"architecture": "hexagonal"`).

### Cross-Module Communication
Chosen once during `justgo new` and stored in `.justgo.json`:
*   **Direct Synchronous Calls** (default): modules call each other's public `Service` interface directly, in-process.
*   **In-Memory Dispatcher**: an in-process pub/sub bus (`pkg/bus/bus.go`, Go channels) for decoupled, asynchronous module-to-module events.
*   **Watermill**: a real message broker (RabbitMQ or Kafka, `pkg/bus/`) for durable, cross-process eventing — useful if you expect to extract modules into separate services later.

Every generated module's `factory.Build(...)` and `module.New(...)` already accept the chosen bus (or `db`) as a parameter so wiring stays consistent; publishing/subscribing to actual domain events is left to you to add in the module's service layer.

### Extract a Module into a Microservice (Experimental)
```bash
./justgo extract <module> [--out=<dir>]

# Example:
./justgo extract billing --out=./billing-service
```
Copies `modules/<name>/` (plus its `pkg/database` / `pkg/bus` dependencies and `db/modules/<name>/`) into a new standalone directory, rewrites its import paths to a new Go module, bootstraps a fresh `cmd/<name>/main.go` + `.justgo.json` + `Dockerfile`/`docker-compose.yml`/`Makefile`, and runs `go mod init` / `go get` / `go mod tidy` so it builds independently. If the module still imports a sibling module (e.g. via a local cross-module interface), `extract` prints a warning listing every remaining reference for you to resolve by hand — it does not attempt to auto-resolve cross-module dependencies.

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
│   ├── routes.tmpl
│   └── hex_main.go.tmpl, hex_handler.tmpl, hex_module.tmpl  # Hexagonal architecture
├── Makefile.tmpl
├── model.tmpl
├── repository.tmpl
├── usecase.tmpl
├── init.tmpl
├── env.tmpl
├── config.tmpl
└── hex_public.tmpl, hex_ports.tmpl, hex_service.tmpl,       # Hexagonal architecture
    hex_repository.tmpl, hex_factory.tmpl, hex_schema.tmpl,  # (router-independent)
    hex_query.tmpl, hex_sqlc.tmpl, bus_inmemory.tmpl,
    bus_watermill_common.tmpl, bus_watermill_rabbitmq.tmpl,
    bus_watermill_kafka.tmpl
```
You can use Go's `text/template` syntax inside these templates. The templates receive a configuration struct with the following fields (booleans unless noted):
*   `.UseDB`: True if database scaffolding is enabled.
*   `.UseObs`: True if the observability stack is enabled.
*   `.Architecture` *(string)*: `"standard"` or `"hexagonal"`.
*   `.MessageBroker` *(string, hexagonal only)*: `"direct"`, `"inmemory"`, or `"watermill"`.
*   `.BrokerBackend` *(string, hexagonal + watermill only)*: `"rabbitmq"` or `"kafka"`.

The `hex_*` templates additionally receive per-module fields when rendered via `justgo gen module`: `.ModuleName` (lowercase), `.ModuleCamel` (CamelCase), and `.ModulePath` (the project's Go module path).
