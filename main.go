package main

import (
	"bufio"
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
)

//go:embed default_structure.txt
var defaultStructure string

//go:embed default_structure_hexagonal.txt
var defaultStructureHexagonal string

//go:embed templates/*
var templatesFS embed.FS

type JustGoConfig struct {
	ProjectName   string `json:"projectName"`
	Router        string `json:"router"`
	UseDB         bool   `json:"useDB"`
	DBEngine      string `json:"dbEngine"`
	UseObs        bool   `json:"useObs"`
	Architecture  string `json:"architecture"`
	MessageBroker string `json:"messageBroker"`
	BrokerBackend string `json:"brokerBackend"`
}

func readTemplateContent(router, name string) (string, error) {
	// 1. Try local project templates/<router>/<name>.tmpl
	localRouterPath := filepath.Join("templates", router, name+".tmpl")
	if _, err := os.Stat(localRouterPath); err == nil {
		content, err := os.ReadFile(localRouterPath)
		if err == nil {
			return string(content), nil
		}
	}
	// 2. Try local project templates/<name>.tmpl
	localPath := filepath.Join("templates", name+".tmpl")
	if _, err := os.Stat(localPath); err == nil {
		content, err := os.ReadFile(localPath)
		if err == nil {
			return string(content), nil
		}
	}
	// 3. Try embedded templates/<router>/<name>.tmpl
	embedRouterPath := "templates/" + router + "/" + name + ".tmpl"
	content, err := templatesFS.ReadFile(embedRouterPath)
	if err == nil {
		return string(content), nil
	}
	// 4. Try embedded templates/<name>.tmpl
	embedPath := "templates/" + name + ".tmpl"
	content, err = templatesFS.ReadFile(embedPath)
	if err == nil {
		return string(content), nil
	}

	return "", fmt.Errorf("template not found: %s (router: %s)", name, router)
}

func loadProjectConfig(root string) (*JustGoConfig, error) {
	configPath := filepath.Join(root, ".justgo.json")
	content, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}
	var cfg JustGoConfig
	if err := json.Unmarshal(content, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

type ProjectConfig struct {
	ProjectName   string
	ModulePath    string
	Router        string
	UseDB         bool
	DBEngine      string
	UseObs        bool
	Architecture  string
	MessageBroker string
	BrokerBackend string
}

type DomainConfig struct {
	ProjectName string
	ModulePath  string
	DomainName  string
	DomainCamel string
	DomainLower string
	UseDB       bool
}

// HexModuleConfig carries per-module template data for the hexagonal
// modular-monolith architecture (see `justgo gen module`).
type HexModuleConfig struct {
	ProjectName   string
	ModulePath    string
	ModuleName    string
	ModuleCamel   string
	UseDB         bool
	DBEngine      string
	MessageBroker string
	BrokerBackend string
}

func main() {
	if len(os.Args) < 2 {
		showHelp()
		return
	}

	subcommand := os.Args[1]
	switch subcommand {
	case "new":
		runNewProjectFlow()
	case "gen":
		if len(os.Args) < 3 {
			fmt.Println("Error: please specify a domain name or a layer. Example:")
			fmt.Println("  justgo gen billing")
			fmt.Println("  justgo gen handler billing Create")
			return
		}
		if os.Args[2] == "module" {
			if len(os.Args) < 4 {
				fmt.Println("Error: please specify a module name. Example:")
				fmt.Println("  justgo gen module billing")
				return
			}
			runGenHexModuleFlow(os.Args[3])
		} else if len(os.Args) == 3 {
			runGenModuleFlow(os.Args[2])
		} else {
			runGranularGenFlow(os.Args[2], os.Args[3], os.Args[4:])
		}
	case "agents":
		runAgentsFlow()
	case "extract":
		if len(os.Args) < 3 {
			fmt.Println("Error: please specify a module name. Example:")
			fmt.Println("  justgo extract billing")
			return
		}
		runExtractModuleFlow(os.Args[2:])
	default:
		showHelp()
	}
}

func showHelp() {
	fmt.Println("==========================================================")
	fmt.Println("      JustGo CLI - Go Scaffolding & Code Generation Tool  ")
	fmt.Println("==========================================================")
	fmt.Println("\nUsage:")
	fmt.Println("  justgo <command> [arguments]")
	fmt.Println("\nAvailable Commands:")
	fmt.Println("  new                         Interactively scaffold a new Go project layout")
	fmt.Println("  gen <domain>                Generate all layers for a domain module (full code gen)")
	fmt.Println("  gen <layer> <domain>        Generate only a specific layer for a domain module")
	fmt.Println("  gen handler <domain> <act>  Append an endpoint method and register its route")
	fmt.Println("  gen module <name>           Generate a bounded-context module (Hexagonal architecture only)")
	fmt.Println("  extract <module>            Experimental: extract a module into a standalone microservice")
	fmt.Println("  agents                      Generate AI agent/harness instructions (AGENTS.md, Claude Skill, Kiro steering)")
	fmt.Println("\nSupported Layers:")
	fmt.Println("  model, repository, usecase, handler, init, routes")
	fmt.Println("\nHandler Append Options:")
	fmt.Println("  --method=<METHOD>           HTTP Method (e.g. GET, POST, PUT, DELETE) [default: GET]")
	fmt.Println("  --path=<PATH>               Route path [default: /<domain>/<action>]")
	fmt.Println("\nExtract Options:")
	fmt.Println("  --out=<DIR>                 Target directory [default: ./<module>-service]")
	fmt.Println("\nExamples:")
	fmt.Println("  justgo new")
	fmt.Println("  justgo gen billing")
	fmt.Println("  justgo gen model billing")
	fmt.Println("  justgo gen handler billing Create --method=POST --path=/api/v1/billing")
	fmt.Println("  justgo gen module billing")
	fmt.Println("  justgo extract billing --out=./billing-service")
	fmt.Println("  justgo agents")
	fmt.Println("\nNote: Mocks are automatically generated using mockgen (run 'make mock' to regenerate).")
	fmt.Println("==========================================================")
}

// defaultDependencies returns the packages justgo installs automatically
// based on the chosen router/DB/observability/messaging options — shared by
// runNewProjectFlow and runExtractModuleFlow so a re-installed dependency
// set never drifts between the two.
func defaultDependencies(router string, useDB bool, dbEngine string, useObs bool, messageBroker, brokerBackend string) []string {
	var dependencies []string

	dependencies = append(dependencies, "github.com/joho/godotenv")
	if router == "gin" {
		dependencies = append(dependencies, "github.com/gin-gonic/gin")
	} else if router == "fiber" {
		dependencies = append(dependencies, "github.com/gofiber/fiber/v3")
	}

	if useObs {
		dependencies = append(dependencies, "github.com/goleggo/observer@v0.1.4")
	}

	if useDB {
		switch dbEngine {
		case "postgres":
			dependencies = append(dependencies, "github.com/jackc/pgx/v5")
		case "mysql":
			dependencies = append(dependencies, "github.com/go-sql-driver/mysql")
		case "sqlite":
			dependencies = append(dependencies, "github.com/mattn/go-sqlite3")
		}
	}

	if messageBroker == "watermill" {
		dependencies = append(dependencies, "github.com/ThreeDotsLabs/watermill")
		switch brokerBackend {
		case "rabbitmq":
			dependencies = append(dependencies, "github.com/ThreeDotsLabs/watermill-amqp/v3")
		case "kafka":
			dependencies = append(dependencies, "github.com/ThreeDotsLabs/watermill-kafka/v3")
		}
	}

	return dependencies
}

func runNewProjectFlow() {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("==================================================")
	fmt.Println("         Welcome to JustGo Project Generator       ")
	fmt.Println("==================================================")

	// 1. Prompt Project Name
	var projectName string
	for {
		fmt.Print("Enter Project Name: ")
		input, _ := reader.ReadString('\n')
		projectName = strings.TrimSpace(input)
		if projectName != "" {
			matched, _ := regexp.MatchString(`^[a-zA-Z0-9-_]+$`, projectName)
			if matched {
				break
			}
			fmt.Println("Invalid project name. Use only alphanumeric characters, hyphens, and underscores.")
		} else {
			fmt.Println("Project name cannot be empty.")
		}
	}

	// 2. Detect System Go Version as default
	defaultGoVer := "1.25.10"
	cmdVer := exec.Command("go", "version")
	if out, err := cmdVer.Output(); err == nil {
		re := regexp.MustCompile(`go([0-9]+\.[0-9]+(\.[0-9]+)?)`)
		matches := re.FindStringSubmatch(string(out))
		if len(matches) > 1 {
			defaultGoVer = matches[1]
		}
	}

	fmt.Printf("Enter Go Version (default: %s): ", defaultGoVer)
	inputVer, _ := reader.ReadString('\n')
	goVersion := strings.TrimSpace(inputVer)
	if goVersion == "" {
		goVersion = defaultGoVer
	}

	// 3. Prompt Router
	var router string
	for {
		fmt.Println("Choose HTTP Router/Framework:")
		fmt.Println("  1. Gin (default)")
		fmt.Println("  2. Fiber v3")
		fmt.Println("  3. Standard Library")
		fmt.Print("Enter choice [1-3]: ")
		inputRouter, _ := reader.ReadString('\n')
		choice := strings.TrimSpace(inputRouter)
		if choice == "" || choice == "1" {
			router = "gin"
			break
		} else if choice == "2" {
			router = "fiber"
			break
		} else if choice == "3" {
			router = "std"
			break
		}
		fmt.Println("Invalid choice. Please enter 1, 2, or 3.")
	}

	// 3.5 Prompt Architecture Style
	var architecture string
	for {
		fmt.Println("Choose Architecture Style:")
		fmt.Println("  1. Standard Clean Architecture (default)")
		fmt.Println("  2. Modular Monolith (Hexagonal)")
		fmt.Print("Enter choice [1-2]: ")
		inputArch, _ := reader.ReadString('\n')
		archChoice := strings.TrimSpace(inputArch)
		if archChoice == "" || archChoice == "1" {
			architecture = "standard"
			break
		} else if archChoice == "2" {
			architecture = "hexagonal"
			break
		}
		fmt.Println("Invalid choice. Please enter 1 or 2.")
	}

	// 4. Prompt Database Scaffolding
	var useDB bool
	var dbEngine string
	fmt.Print("Enable Database Scaffolding? (y/N): ")
	inputDB, _ := reader.ReadString('\n')
	dbChoice := strings.ToLower(strings.TrimSpace(inputDB))
	if dbChoice == "y" || dbChoice == "yes" {
		useDB = true
		for {
			fmt.Println("Choose Database Engine:")
			fmt.Println("  1. PostgreSQL (default)")
			fmt.Println("  2. MySQL")
			fmt.Println("  3. SQLite")
			fmt.Print("Enter choice [1-3]: ")
			inputEngine, _ := reader.ReadString('\n')
			engineChoice := strings.TrimSpace(inputEngine)
			if engineChoice == "" || engineChoice == "1" {
				dbEngine = "postgres"
				break
			} else if engineChoice == "2" {
				dbEngine = "mysql"
				break
			} else if engineChoice == "3" {
				dbEngine = "sqlite"
				break
			}
			fmt.Println("Invalid choice. Please enter 1, 2, or 3.")
		}
	}

	// 4.5 Prompt Observability
	var useObs bool
	fmt.Print("Enable Observability Stack (github.com/goleggo/observer)? (Y/n): ")
	inputObs, _ := reader.ReadString('\n')
	obsChoice := strings.ToLower(strings.TrimSpace(inputObs))
	if obsChoice == "" || obsChoice == "y" || obsChoice == "yes" {
		useObs = true
	}

	// 4.75 Prompt Cross-Module Messaging (hexagonal architecture only)
	var messageBroker string
	var brokerBackend string
	if architecture == "hexagonal" {
		for {
			fmt.Println("Choose Cross-Module Communication:")
			fmt.Println("  1. Direct Synchronous Calls (default)")
			fmt.Println("  2. In-Memory Dispatcher (Go channels)")
			fmt.Println("  3. Watermill (message broker)")
			fmt.Print("Enter choice [1-3]: ")
			inputBroker, _ := reader.ReadString('\n')
			brokerChoice := strings.TrimSpace(inputBroker)
			if brokerChoice == "" || brokerChoice == "1" {
				messageBroker = "direct"
				break
			} else if brokerChoice == "2" {
				messageBroker = "inmemory"
				break
			} else if brokerChoice == "3" {
				messageBroker = "watermill"
				break
			}
			fmt.Println("Invalid choice. Please enter 1, 2, or 3.")
		}

		if messageBroker == "watermill" {
			for {
				fmt.Println("Choose Message Broker:")
				fmt.Println("  1. RabbitMQ (default)")
				fmt.Println("  2. Kafka")
				fmt.Print("Enter choice [1-2]: ")
				inputBackend, _ := reader.ReadString('\n')
				backendChoice := strings.TrimSpace(inputBackend)
				if backendChoice == "" || backendChoice == "1" {
					brokerBackend = "rabbitmq"
					break
				} else if backendChoice == "2" {
					brokerBackend = "kafka"
					break
				}
				fmt.Println("Invalid choice. Please enter 1 or 2.")
			}
		}
	}

	// 5. Prompt Dependencies
	fmt.Print("Enter Dependencies (space-separated, e.g. github.com/joho/godotenv): ")
	inputDeps, _ := reader.ReadString('\n')
	depsStr := strings.TrimSpace(inputDeps)
	var dependencies []string
	if depsStr != "" {
		dependencies = strings.Fields(depsStr)
	}

	// Add default packages automatically (router, observability, DB driver, messaging broker)
	dependencies = append(dependencies, defaultDependencies(router, useDB, dbEngine, useObs, messageBroker, brokerBackend)...)

	// 6. Confirm Settings
	fmt.Println("\nConfiguration Summary:")
	fmt.Printf("  - Project Name : %s\n", projectName)
	fmt.Printf("  - Go Version   : %s\n", goVersion)
	fmt.Printf("  - HTTP Router  : %s\n", router)
	if architecture == "hexagonal" {
		fmt.Println("  - Architecture : Modular Monolith (Hexagonal)")
	} else {
		fmt.Println("  - Architecture : Standard Clean Architecture")
	}
	if useDB {
		fmt.Printf("  - DB Engine    : %s (sqlc)\n", dbEngine)
	} else {
		fmt.Println("  - DB Scaffolding: Disabled")
	}
	if useObs {
		fmt.Println("  - Observability: Enabled (github.com/goleggo/observer@v0.1.4)")
	} else {
		fmt.Println("  - Observability: Disabled")
	}
	if architecture == "hexagonal" {
		switch messageBroker {
		case "inmemory":
			fmt.Println("  - Messaging    : In-Memory Dispatcher")
		case "watermill":
			fmt.Printf("  - Messaging    : Watermill (%s)\n", brokerBackend)
		default:
			fmt.Println("  - Messaging    : Direct Synchronous Calls")
		}
	}
	if len(dependencies) > 0 {
		fmt.Printf("  - Dependencies : %s\n", strings.Join(dependencies, ", "))
	} else {
		fmt.Println("  - Dependencies : None")
	}
	fmt.Printf("  - Target Dir   : %s\n", filepath.Join(".", projectName))
	fmt.Print("\nDo you want to generate this project? (Y/n): ")
	confirmInput, _ := reader.ReadString('\n')
	confirm := strings.ToLower(strings.TrimSpace(confirmInput))
	if confirm != "" && confirm != "y" && confirm != "yes" {
		fmt.Println("Generation cancelled.")
		return
	}

	// 7. Generate Project
	fmt.Printf("\nGenerating project '%s'...\n", projectName)
	err := generateProject(projectName, goVersion, router, useDB, dbEngine, useObs, architecture, messageBroker, brokerBackend, dependencies)
	if err != nil {
		fmt.Printf("Error generating project: %v\n", err)
		return
	}

	fmt.Println("\n==================================================")
	fmt.Printf(" Success! Project '%s' generated successfully.\n", projectName)
	fmt.Println(" Run the following commands to get started:")
	fmt.Printf("   cd %s\n", projectName)
	fmt.Println("   make run")
	fmt.Println("==================================================")
}

func runGenModuleFlow(domainName string) {
	domainName = strings.ToLower(strings.TrimSpace(domainName))
	if domainName == "" {
		fmt.Println("Error: invalid domain name")
		return
	}

	root, modulePath, projectName, err := detectProjectRoot()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	router := "gin" // fallback default
	useDB := false
	if cfg, err := loadProjectConfig(root); err == nil {
		router = cfg.Router
		useDB = cfg.UseDB
	}

	fmt.Printf("Detected project '%s' (module: '%s', router: '%s') at: %s\n", projectName, modulePath, router, root)
	fmt.Printf("Generating domain module '%s'...\n", domainName)

	config := DomainConfig{
		ProjectName: projectName,
		ModulePath:  modulePath,
		DomainName:  domainName,
		DomainCamel: toCamelCase(domainName),
		DomainLower: domainName,
		UseDB:       useDB,
	}

	domainDir := filepath.Join(root, "internal", domainName)

	// Create directories
	subDirs := []string{"model", "repository", "usecase", "handler"}
	for _, sub := range subDirs {
		dirPath := filepath.Join(domainDir, sub)
		if err := os.MkdirAll(dirPath, 0755); err != nil {
			fmt.Printf("Error creating directory %s: %v\n", dirPath, err)
			return
		}
	}

	// Write templates
	for _, sub := range subDirs {
		filePath := filepath.Join(domainDir, sub, fmt.Sprintf("%s_%s.go", domainName, sub))
		tmplContent, err := readTemplateContent(router, sub)
		if err != nil {
			fmt.Printf("Error loading %s template: %v\n", sub, err)
			return
		}
		
		t, err := template.New(sub).Parse(tmplContent)
		if err != nil {
			fmt.Printf("Error parsing %s template: %v\n", sub, err)
			return
		}

		f, err := os.Create(filePath)
		if err != nil {
			fmt.Printf("Error creating file %s: %v\n", filePath, err)
			return
		}

		err = t.Execute(f, config)
		f.Close()
		if err != nil {
			fmt.Printf("Error writing file %s: %v\n", filePath, err)
			return
		}
		fmt.Printf("Created: %s\n", filepath.Join("internal", domainName, sub, fmt.Sprintf("%s_%s.go", domainName, sub)))
	}

	// Write root files (init.go and routes.go)
	rootFiles := []string{"init", "routes"}
	for _, rf := range rootFiles {
		filePath := filepath.Join(domainDir, fmt.Sprintf("%s.go", rf))
		tmplContent, err := readTemplateContent(router, rf)
		if err != nil {
			fmt.Printf("Error loading %s template: %v\n", rf, err)
			return
		}
		
		t, err := template.New(rf).Parse(tmplContent)
		if err != nil {
			fmt.Printf("Error parsing %s template: %v\n", rf, err)
			return
		}

		f, err := os.Create(filePath)
		if err != nil {
			fmt.Printf("Error creating file %s: %v\n", filePath, err)
			return
		}

		err = t.Execute(f, config)
		f.Close()
		if err != nil {
			fmt.Printf("Error writing file %s: %v\n", filePath, err)
			return
		}
		fmt.Printf("Created: %s\n", filepath.Join("internal", domainName, fmt.Sprintf("%s.go", rf)))
	}

	// Inject wiring
	fmt.Println("Wiring up routes and imports...")
	if err := injectCodegenMarkers(root, projectName, modulePath, domainName, router); err != nil {
		fmt.Printf("Error wiring up: %v\n", err)
		return
	}

	fmt.Println("Running go mod tidy...")
	cmdTidy := exec.Command("go", "mod", "tidy")
	cmdTidy.Dir = root
	_ = cmdTidy.Run()

	runMockgen(root, modulePath, domainName, config.DomainCamel)

	fmt.Println("Done! Domain module generated and wired up successfully.")
}

const claudeSkillFrontmatter = `---
name: justgo-workflow
description: Use when adding domains, layers, or HTTP endpoints in this justgo-scaffolded project, or touching internal/* generated code, main.go codegen markers, or mocks — explains justgo CLI commands and conventions instead of hand-editing.
---

`

const kiroSteeringFrontmatter = `---
inclusion: always
---

`

func writeAgentsDoc(path, content string, config ProjectConfig) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create directory for %s: %w", path, err)
	}

	t, err := template.New("agents").Parse(content)
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %w", path, err)
	}
	defer f.Close()

	if err := t.Execute(f, config); err != nil {
		return fmt.Errorf("failed to execute template on %s: %w", path, err)
	}

	return nil
}

func runAgentsFlow() {
	root, modulePath, projectName, err := detectProjectRoot()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	router := "gin"
	useDB := false
	dbEngine := ""
	useObs := false
	if cfg, err := loadProjectConfig(root); err == nil {
		router = cfg.Router
		useDB = cfg.UseDB
		dbEngine = cfg.DBEngine
		useObs = cfg.UseObs
	}

	fmt.Printf("Detected project '%s' (module: '%s', router: '%s') at: %s\n", projectName, modulePath, router, root)

	reader := bufio.NewReader(os.Stdin)
	fmt.Println("\nWhich AI agent/harness docs do you want to generate?")
	fmt.Println("  1. AGENTS.md (universal: Claude Code, Codex, Cursor, VS Code, ...)")
	fmt.Println("  2. Claude Code Skill (.claude/skills/justgo-workflow/SKILL.md)")
	fmt.Println("  3. Kiro steering (.kiro/steering/justgo-workflow.md)")
	fmt.Println("  4. All of the above")
	fmt.Print("Enter choice(s), comma-separated [default: 1]: ")
	input, _ := reader.ReadString('\n')
	choice := strings.TrimSpace(input)
	if choice == "" {
		choice = "1"
	}

	selected := map[string]bool{}
	for _, part := range strings.Split(choice, ",") {
		switch strings.TrimSpace(part) {
		case "1":
			selected["agents"] = true
		case "2":
			selected["skill"] = true
		case "3":
			selected["kiro"] = true
		case "4":
			selected["agents"] = true
			selected["skill"] = true
			selected["kiro"] = true
		}
	}
	if len(selected) == 0 {
		fmt.Println("No valid choice selected. Nothing generated.")
		return
	}

	body, err := readTemplateContent(router, "agents_body")
	if err != nil {
		fmt.Printf("Error loading agents body template: %v\n", err)
		return
	}

	config := ProjectConfig{
		ProjectName: projectName,
		ModulePath:  modulePath,
		Router:      router,
		UseDB:       useDB,
		DBEngine:    dbEngine,
		UseObs:      useObs,
	}

	if selected["agents"] {
		path := filepath.Join(root, "AGENTS.md")
		if err := writeAgentsDoc(path, body, config); err != nil {
			fmt.Printf("Error writing AGENTS.md: %v\n", err)
		} else {
			fmt.Printf("Created: %s\n", path)
		}
	}
	if selected["skill"] {
		path := filepath.Join(root, ".claude", "skills", "justgo-workflow", "SKILL.md")
		if err := writeAgentsDoc(path, claudeSkillFrontmatter+body, config); err != nil {
			fmt.Printf("Error writing Claude Code Skill: %v\n", err)
		} else {
			fmt.Printf("Created: %s\n", path)
		}
	}
	if selected["kiro"] {
		path := filepath.Join(root, ".kiro", "steering", "justgo-workflow.md")
		if err := writeAgentsDoc(path, kiroSteeringFrontmatter+body, config); err != nil {
			fmt.Printf("Error writing Kiro steering doc: %v\n", err)
		} else {
			fmt.Printf("Created: %s\n", path)
		}
	}

	fmt.Println("Done!")
}

// runGenHexModuleFlow scaffolds a bounded-context module for the Modular
// Monolith (Hexagonal) architecture: a public API file, a private
// internal/{ports,service,adapter} tree, a factory/ DI wiring package, and a
// module.go wrapper — see justgo/README.md for the full rationale.
func runGenHexModuleFlow(moduleName string) {
	moduleName = strings.ToLower(strings.TrimSpace(moduleName))
	if moduleName == "" {
		fmt.Println("Error: invalid module name")
		return
	}

	root, modulePath, projectName, err := detectProjectRoot()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	cfg, err := loadProjectConfig(root)
	if err != nil {
		fmt.Printf("Error: could not read .justgo.json: %v\n", err)
		return
	}
	if cfg.Architecture != "hexagonal" {
		fmt.Println("Error: this project is not using the Modular Monolith (Hexagonal) architecture.")
		fmt.Println("Use 'justgo gen <domain>' instead, or scaffold a new project with Hexagonal architecture via 'justgo new'.")
		return
	}

	router := cfg.Router
	useDB := cfg.UseDB
	messageBroker := cfg.MessageBroker
	moduleCamel := toCamelCase(moduleName)

	fmt.Printf("Detected project '%s' (module: '%s', router: '%s') at: %s\n", projectName, modulePath, router, root)
	fmt.Printf("Generating hexagonal module '%s'...\n", moduleName)

	config := HexModuleConfig{
		ProjectName:   projectName,
		ModulePath:    modulePath,
		ModuleName:    moduleName,
		ModuleCamel:   moduleCamel,
		UseDB:         useDB,
		DBEngine:      cfg.DBEngine,
		MessageBroker: messageBroker,
		BrokerBackend: cfg.BrokerBackend,
	}

	moduleDir := filepath.Join(root, "modules", moduleName)
	subDirs := []string{
		filepath.Join("internal", "ports"),
		filepath.Join("internal", "service"),
		filepath.Join("internal", "adapter", "http"),
		filepath.Join("internal", "adapter", "repository"),
		"factory",
		"module",
		"mocks",
	}
	for _, sub := range subDirs {
		dirPath := filepath.Join(moduleDir, sub)
		if err := os.MkdirAll(dirPath, 0755); err != nil {
			fmt.Printf("Error creating directory %s: %v\n", dirPath, err)
			return
		}
	}

	type hexFileSpec struct {
		tmplName string
		destRel  string
	}
	files := []hexFileSpec{
		{"hex_public", moduleName + ".go"},
		{"hex_ports", filepath.Join("internal", "ports", "repository.go")},
		{"hex_service", filepath.Join("internal", "service", moduleName+"_service.go")},
		{"hex_repository", filepath.Join("internal", "adapter", "repository", moduleName+"_repository.go")},
		{"hex_handler", filepath.Join("internal", "adapter", "http", moduleName+"_handler.go")},
		{"hex_factory", filepath.Join("factory", "factory.go")},
		{"hex_module", filepath.Join("module", "module.go")},
	}

	for _, f := range files {
		tmplContent, err := readTemplateContent(router, f.tmplName)
		if err != nil {
			fmt.Printf("Error loading %s template: %v\n", f.tmplName, err)
			return
		}
		fullPath := filepath.Join(moduleDir, f.destRel)
		if err := writeTemplate(fullPath, tmplContent, config); err != nil {
			fmt.Printf("Error writing file %s: %v\n", f.destRel, err)
			return
		}
		fmt.Printf("Created: %s\n", filepath.Join("modules", moduleName, f.destRel))
	}

	// DB scaffolding: each module owns its own schema/queries, wired as its
	// own `sql:` entry in the project's sqlc.yaml.
	if useDB {
		dbModuleDir := filepath.Join(root, "db", "modules", moduleName)
		dbQueriesDir := filepath.Join(dbModuleDir, "queries")
		if err := os.MkdirAll(dbQueriesDir, 0755); err != nil {
			fmt.Printf("Error creating db dir: %v\n", err)
			return
		}

		schemaTmpl, err := readTemplateContent(router, "hex_schema")
		if err == nil {
			if err := writeTemplate(filepath.Join(dbModuleDir, "schema.sql"), schemaTmpl, config); err != nil {
				fmt.Printf("Error writing schema: %v\n", err)
				return
			}
			fmt.Printf("Created: %s\n", filepath.Join("db", "modules", moduleName, "schema.sql"))
		}

		queryTmpl, err := readTemplateContent(router, "hex_query")
		if err == nil {
			if err := writeTemplate(filepath.Join(dbQueriesDir, "query.sql"), queryTmpl, config); err != nil {
				fmt.Printf("Error writing query: %v\n", err)
				return
			}
			fmt.Printf("Created: %s\n", filepath.Join("db", "modules", moduleName, "queries", "query.sql"))
		}

		if err := injectSqlcModule(root, moduleName); err != nil {
			fmt.Printf("Warning: failed to wire sqlc.yaml: %v\n", err)
		} else {
			fmt.Println("Wired module into sqlc.yaml")
		}
	}

	fmt.Println("Wiring up routes and imports...")
	if err := injectHexModuleMarkers(root, projectName, modulePath, moduleName, router, useDB, messageBroker); err != nil {
		fmt.Printf("Error wiring up: %v\n", err)
		return
	}

	fmt.Println("Running go mod tidy...")
	cmdTidy := exec.Command("go", "mod", "tidy")
	cmdTidy.Dir = root
	_ = cmdTidy.Run()

	runHexMockgen(root, moduleName)

	fmt.Println("Done! Hexagonal module generated and wired up successfully.")
}

// injectSqlcModule splices a new `sql:` entry for moduleName into the
// project's sqlc.yaml, directly before the `# [justgo:sqlc]` marker (so the
// marker stays available for the next module), matching the same
// insert-before-marker convention used for `// [justgo:routes]`.
func injectSqlcModule(projectRoot, moduleName string) error {
	sqlcPath := filepath.Join(projectRoot, "sqlc.yaml")
	content, err := os.ReadFile(sqlcPath)
	if err != nil {
		return err
	}

	const marker = "# [justgo:sqlc]"
	strContent := string(content)
	if !strings.Contains(strContent, marker) {
		return fmt.Errorf("marker %q not found in sqlc.yaml", marker)
	}

	entry := fmt.Sprintf(`  - schema: "db/modules/%s/schema.sql"
    queries: "db/modules/%s/queries/"
    gen:
      go:
        package: "sqlcgen"
        out: "modules/%s/internal/adapter/repository/sqlcgen"
        sql_package: "database/sql"
        emit_db_tags: true
        emit_interface: true
  %s`, moduleName, moduleName, moduleName, marker)

	updated := strings.Replace(strContent, "  "+marker, entry, 1)
	return os.WriteFile(sqlcPath, []byte(updated), 0644)
}

// runHexMockgen generates a mock for a hexagonal module's outbound
// Repository port, analogous to runMockgen for the standard architecture.
func runHexMockgen(projectRoot, moduleName string) {
	mockgenPath, err := exec.LookPath("mockgen")
	if err != nil {
		// mockgen is not in PATH, skip silently
		return
	}

	mocksDir := filepath.Join(projectRoot, "modules", moduleName, "mocks")
	if err := os.MkdirAll(mocksDir, 0755); err != nil {
		fmt.Printf("Warning: failed to create mocks directory: %v\n", err)
		return
	}

	portsSource := filepath.Join(projectRoot, "modules", moduleName, "internal", "ports", "repository.go")
	if _, err := os.Stat(portsSource); err != nil {
		return
	}

	dest := filepath.Join(mocksDir, "mock_repository.go")
	cmd := exec.Command(mockgenPath,
		"-source="+portsSource,
		"-destination="+dest,
		"-package=mocks",
	)
	cmd.Dir = projectRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Printf("Warning: mockgen repository failed: %s\n", string(out))
	} else {
		fmt.Printf("Generated mock: %s\n", filepath.Join("modules", moduleName, "mocks", "mock_repository.go"))
	}
}

// runExtractModuleFlow is an experimental command that copies a hexagonal
// module out of its parent project into a standalone, independently
// buildable service directory. It does not attempt to resolve dependencies
// on sibling modules — those are flagged for the user to fix by hand.
func runExtractModuleFlow(args []string) {
	moduleName := strings.ToLower(strings.TrimSpace(args[0]))
	if moduleName == "" {
		fmt.Println("Error: invalid module name")
		return
	}

	outDir := "./" + moduleName + "-service"
	for _, arg := range args[1:] {
		if strings.HasPrefix(arg, "--out=") {
			outDir = strings.TrimPrefix(arg, "--out=")
		}
	}

	root, modulePath, projectName, err := detectProjectRoot()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	cfg, err := loadProjectConfig(root)
	if err != nil {
		fmt.Printf("Error: could not read .justgo.json: %v\n", err)
		return
	}
	if cfg.Architecture != "hexagonal" {
		fmt.Println("Error: 'extract' is only supported for Modular Monolith (Hexagonal) projects.")
		return
	}

	moduleSrcDir := filepath.Join(root, "modules", moduleName)
	if _, err := os.Stat(moduleSrcDir); err != nil {
		fmt.Printf("Error: module '%s' not found at %s\n", moduleName, moduleSrcDir)
		return
	}

	absOut, err := filepath.Abs(outDir)
	if err != nil {
		fmt.Printf("Error resolving output directory: %v\n", err)
		return
	}
	newModuleName := filepath.Base(absOut)

	if err := os.MkdirAll(absOut, 0755); err != nil {
		fmt.Printf("Error creating output directory: %v\n", err)
		return
	}

	fmt.Printf("Extracting module '%s' from '%s' into '%s' (experimental)...\n", moduleName, projectName, absOut)

	// 1. Copy the module itself, keeping the same modules/<name> relative
	// shape so the internal/ Go-enforced boundary keeps working unmodified.
	if err := copyDir(moduleSrcDir, filepath.Join(absOut, "modules", moduleName)); err != nil {
		fmt.Printf("Error copying module: %v\n", err)
		return
	}

	// 2. Copy shared pkg/ dependencies this module may rely on.
	if cfg.UseDB {
		_ = copyDir(filepath.Join(root, "pkg", "database"), filepath.Join(absOut, "pkg", "database"))
	}
	if cfg.MessageBroker != "" && cfg.MessageBroker != "direct" {
		_ = copyDir(filepath.Join(root, "pkg", "bus"), filepath.Join(absOut, "pkg", "bus"))
	}

	// 3. Copy this module's own DB schema/queries, if any.
	if cfg.UseDB {
		dbModuleSrc := filepath.Join(root, "db", "modules", moduleName)
		if _, err := os.Stat(dbModuleSrc); err == nil {
			_ = copyDir(dbModuleSrc, filepath.Join(absOut, "db", "modules", moduleName))
		}
	}

	// 4. Rewrite import paths from the parent module path to the new one.
	if err := rewriteImports(absOut, modulePath, newModuleName); err != nil {
		fmt.Printf("Error rewriting imports: %v\n", err)
		return
	}

	// 5. Flag any remaining sibling-module imports for manual resolution.
	warnSiblingModuleImports(absOut, newModuleName, moduleName)

	// 6. Re-render project-level scaffolding for the new standalone service.
	projConfig := ProjectConfig{
		ProjectName:   newModuleName,
		ModulePath:    newModuleName,
		Router:        cfg.Router,
		UseDB:         cfg.UseDB,
		DBEngine:      cfg.DBEngine,
		UseObs:        cfg.UseObs,
		Architecture:  "hexagonal",
		MessageBroker: cfg.MessageBroker,
		BrokerBackend: cfg.BrokerBackend,
	}

	configDir := filepath.Join(absOut, "internal", "config")
	if err := os.MkdirAll(configDir, 0755); err == nil {
		if tmpl, err := readTemplateContent(cfg.Router, "config"); err == nil {
			_ = writeTemplate(filepath.Join(configDir, "config.go"), tmpl, projConfig)
		}
		if tmpl, err := readTemplateContent(cfg.Router, "env"); err == nil {
			_ = writeTemplate(filepath.Join(absOut, ".env"), tmpl, projConfig)
		}
	}
	if tmpl, err := readTemplateContent(cfg.Router, "dockerfile"); err == nil {
		_ = writeTemplate(filepath.Join(absOut, "Dockerfile"), tmpl, projConfig)
	}
	if tmpl, err := readTemplateContent(cfg.Router, "docker_compose"); err == nil {
		_ = writeTemplate(filepath.Join(absOut, "docker-compose.yml"), tmpl, projConfig)
	}
	if tmpl, err := readTemplateContent(cfg.Router, "Makefile"); err == nil {
		_ = writeTemplate(filepath.Join(absOut, "Makefile"), tmpl, projConfig)
	}

	newCfg := JustGoConfig{
		ProjectName:   newModuleName,
		Router:        cfg.Router,
		UseDB:         cfg.UseDB,
		DBEngine:      cfg.DBEngine,
		UseObs:        cfg.UseObs,
		Architecture:  "hexagonal",
		MessageBroker: cfg.MessageBroker,
		BrokerBackend: cfg.BrokerBackend,
	}
	if cfgBytes, err := json.MarshalIndent(newCfg, "", "  "); err == nil {
		_ = os.WriteFile(filepath.Join(absOut, ".justgo.json"), cfgBytes, 0644)
	}

	// 7. Bootstrap cmd/<newModuleName>/main.go from the same hex_main.go
	// template `justgo new` uses, then wire the single module in through the
	// standard marker-injection path — this also means `justgo gen module`
	// keeps working in the extracted repo if more modules are added later.
	cmdDir := filepath.Join(absOut, "cmd", newModuleName)
	if err := os.MkdirAll(cmdDir, 0755); err != nil {
		fmt.Printf("Error creating cmd dir: %v\n", err)
		return
	}
	mainTmpl, err := readTemplateContent(cfg.Router, "hex_main.go")
	if err != nil {
		fmt.Printf("Error loading main.go template: %v\n", err)
		return
	}
	if err := writeTemplate(filepath.Join(cmdDir, "main.go"), mainTmpl, projConfig); err != nil {
		fmt.Printf("Error writing main.go: %v\n", err)
		return
	}
	if err := injectHexModuleMarkers(absOut, newModuleName, newModuleName, moduleName, cfg.Router, cfg.UseDB, cfg.MessageBroker); err != nil {
		fmt.Printf("Error wiring module: %v\n", err)
		return
	}

	// 8. go mod init / get / tidy
	fmt.Println("Initializing Go module...")
	cmdInit := exec.Command("go", "mod", "init", newModuleName)
	cmdInit.Dir = absOut
	if out, err := cmdInit.CombinedOutput(); err != nil {
		fmt.Printf("Error: go mod init failed: %s: %v\n", string(out), err)
		return
	}

	deps := defaultDependencies(cfg.Router, cfg.UseDB, cfg.DBEngine, cfg.UseObs, cfg.MessageBroker, cfg.BrokerBackend)
	if len(deps) > 0 {
		fmt.Println("Installing dependencies...")
		for _, dep := range deps {
			fmt.Printf("  go get %s...\n", dep)
			cmdGet := exec.Command("go", "get", dep)
			cmdGet.Dir = absOut
			if out, err := cmdGet.CombinedOutput(); err != nil {
				fmt.Printf("Warning: failed to get %s: %s\n", dep, string(out))
			}
		}
	}

	fmt.Println("Tidying up Go module...")
	cmdTidy := exec.Command("go", "mod", "tidy")
	cmdTidy.Dir = absOut
	_ = cmdTidy.Run()

	fmt.Printf("\nDone (experimental)! Extracted module now lives at: %s\n", absOut)
	fmt.Println("Review the warnings above (if any) for remaining cross-module dependencies before running.")
}

// copyDir recursively copies srcDir into dstDir, creating directories as needed.
func copyDir(srcDir, dstDir string) error {
	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		dstPath := filepath.Join(dstDir, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, 0755)
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dstPath, content, 0644)
	})
}

// rewriteImports replaces every occurrence of oldModulePath with
// newModuleName across all .go files under dir — a plain string-splice pass
// (consistent with the marker-injection style used elsewhere in justgo)
// rather than a full AST rewrite.
func rewriteImports(dir, oldModulePath, newModuleName string) error {
	if oldModulePath == "" || oldModulePath == newModuleName {
		return nil
	}
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || filepath.Ext(path) != ".go" {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		updated := strings.ReplaceAll(string(content), oldModulePath, newModuleName)
		if updated == string(content) {
			return nil
		}
		return os.WriteFile(path, []byte(updated), 0644)
	})
}

// warnSiblingModuleImports scans the copied code for imports of any other
// modules/<other> package — justgo doesn't track cross-module Go
// dependencies, so these must be resolved by hand.
func warnSiblingModuleImports(dir, newModuleName, extractedModule string) {
	siblingPrefix := newModuleName + "/modules/"
	ownPrefix := newModuleName + "/modules/" + extractedModule
	var found []string

	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || filepath.Ext(path) != ".go" {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		for _, line := range strings.Split(string(content), "\n") {
			if strings.Contains(line, siblingPrefix) && !strings.Contains(line, ownPrefix) {
				found = append(found, fmt.Sprintf("%s: %s", path, strings.TrimSpace(line)))
			}
		}
		return nil
	})

	if len(found) > 0 {
		fmt.Println("\nWarning: the extracted module still imports other module(s) from the parent project.")
		fmt.Println("justgo does not track cross-module Go dependencies — resolve these manually:")
		for _, f := range found {
			fmt.Printf("  %s\n", f)
		}
	}
}

func generateProject(projectName, goVersion, router string, useDB bool, dbEngine string, useObs bool, architecture, messageBroker, brokerBackend string, dependencies []string) error {
	projectDir := filepath.Join(".", projectName)
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		return fmt.Errorf("failed to create root dir: %w", err)
	}

	isHexagonal := architecture == "hexagonal"

	// Save config
	cfg := JustGoConfig{
		ProjectName:   projectName,
		Router:        router,
		UseDB:         useDB,
		DBEngine:      dbEngine,
		UseObs:        useObs,
		Architecture:  architecture,
		MessageBroker: messageBroker,
		BrokerBackend: brokerBackend,
	}
	cfgBytes, err := json.MarshalIndent(cfg, "", "  ")
	if err == nil {
		_ = os.WriteFile(filepath.Join(projectDir, ".justgo.json"), cfgBytes, 0644)
	}

	projConfig := ProjectConfig{
		ProjectName:   projectName,
		ModulePath:    projectName,
		Router:        router,
		UseDB:         useDB,
		DBEngine:      dbEngine,
		UseObs:        useObs,
		Architecture:  architecture,
		MessageBroker: messageBroker,
		BrokerBackend: brokerBackend,
	}

	// Create internal/config/ config.go file and .env file
	configDir := filepath.Join(projectDir, "internal", "config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config dir: %w", err)
	}
	configTmpl, err := readTemplateContent(router, "config")
	if err != nil {
		return err
	}
	if err := writeTemplate(filepath.Join(configDir, "config.go"), configTmpl, projConfig); err != nil {
		return err
	}
	envTmpl, err := readTemplateContent(router, "env")
	if err != nil {
		return err
	}
	if err := writeTemplate(filepath.Join(projectDir, ".env"), envTmpl, projConfig); err != nil {
		return err
	}

	// Create database & sqlc files if useDB is enabled
	if useDB {
		dbDir := filepath.Join(projectDir, "pkg", "database")
		if err := os.MkdirAll(dbDir, 0755); err != nil {
			return fmt.Errorf("failed to create db dir: %w", err)
		}
		dbTmpl, err := readTemplateContent(router, "database")
		if err == nil {
			if err := writeTemplate(filepath.Join(dbDir, "db.go"), dbTmpl, projConfig); err != nil {
				return err
			}
		}

		// Create db/migrations/ directory (shared across both architectures)
		dbMigrationsDir := filepath.Join(projectDir, "db", "migrations")
		if err := os.MkdirAll(dbMigrationsDir, 0755); err != nil {
			return fmt.Errorf("failed to create db migrations dir: %w", err)
		}
		// Create a placeholder .gitkeep in migrations
		_ = os.WriteFile(filepath.Join(dbMigrationsDir, ".gitkeep"), []byte(""), 0644)

		if isHexagonal {
			// Hexagonal: each module owns its own schema/queries under
			// db/modules/<name>/, wired into sqlc.yaml as they're generated
			// via `justgo gen module`. Start with an empty `sql:` list.
			hexSqlcTmpl, err := readTemplateContent(router, "hex_sqlc")
			if err == nil {
				if err := writeTemplate(filepath.Join(projectDir, "sqlc.yaml"), hexSqlcTmpl, projConfig); err != nil {
					return err
				}
			}
		} else {
			dbSchemaDir := filepath.Join(projectDir, "db")
			dbQueriesDir := filepath.Join(projectDir, "db", "queries")
			if err := os.MkdirAll(dbQueriesDir, 0755); err != nil {
				return fmt.Errorf("failed to create db queries dir: %w", err)
			}

			sqlcTmpl, err := readTemplateContent(router, "sqlc")
			if err == nil {
				if err := writeTemplate(filepath.Join(projectDir, "sqlc.yaml"), sqlcTmpl, projConfig); err != nil {
					return err
				}
			}
			schemaTmpl, err := readTemplateContent(router, "schema")
			if err == nil {
				if err := writeTemplate(filepath.Join(dbSchemaDir, "schema.sql"), schemaTmpl, projConfig); err != nil {
					return err
				}
			}
			queryTmpl, err := readTemplateContent(router, "query")
			if err == nil {
				if err := writeTemplate(filepath.Join(dbQueriesDir, "query.sql"), queryTmpl, projConfig); err != nil {
					return err
				}
			}
		}
	}

	// Create Dockerfile and docker-compose.yml files
	dockerfileTmpl, err := readTemplateContent(router, "dockerfile")
	if err == nil {
		if err := writeTemplate(filepath.Join(projectDir, "Dockerfile"), dockerfileTmpl, projConfig); err != nil {
			return err
		}
	}
	dockerComposeTmpl, err := readTemplateContent(router, "docker_compose")
	if err == nil {
		if err := writeTemplate(filepath.Join(projectDir, "docker-compose.yml"), dockerComposeTmpl, projConfig); err != nil {
			return err
		}
	}

	// Scaffold the cross-module message bus for hexagonal projects.
	if isHexagonal && messageBroker != "" && messageBroker != "direct" {
		busDir := filepath.Join(projectDir, "pkg", "bus")
		if err := os.MkdirAll(busDir, 0755); err != nil {
			return fmt.Errorf("failed to create bus dir: %w", err)
		}

		busTmplName := "bus_inmemory"
		if messageBroker == "watermill" {
			busTmplName = "bus_watermill_common"
		}
		busTmpl, err := readTemplateContent(router, busTmplName)
		if err != nil {
			return err
		}
		if err := writeTemplate(filepath.Join(busDir, "bus.go"), busTmpl, projConfig); err != nil {
			return err
		}

		if messageBroker == "watermill" {
			backendTmplName := "bus_watermill_rabbitmq"
			if brokerBackend == "kafka" {
				backendTmplName = "bus_watermill_kafka"
			}
			backendTmpl, err := readTemplateContent(router, backendTmplName)
			if err != nil {
				return err
			}
			if err := writeTemplate(filepath.Join(busDir, "watermill_bus.go"), backendTmpl, projConfig); err != nil {
				return err
			}
		}
	}

	structureText := defaultStructure
	if isHexagonal {
		structureText = defaultStructureHexagonal
	}
	lines := strings.Split(structureText, "\n")

	stack := make([]string, 20)
	stack[0] = projectDir

	createdDirs := make(map[string]bool)

	for _, line := range lines {
		name, isDir, level, valid := parseLine(line)
		if !valid {
			continue
		}

		if level == 0 {
			continue
		}

		mappedName, ok := mapName(name, projectName)
		if !ok {
			continue
		}

		parentPath := stack[level-1]
		fullPath := filepath.Join(parentPath, mappedName)

		if isDir {
			stack[level] = fullPath
			if err := os.MkdirAll(fullPath, 0755); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", fullPath, err)
			}
			createdDirs[fullPath] = true
			
			if strings.HasSuffix(filepath.ToSlash(fullPath), "cmd/"+projectName) {
				mainGoPath := filepath.Join(fullPath, "main.go")
				mainTmplName := "main.go"
				if isHexagonal {
					mainTmplName = "hex_main.go"
				}
				tmpl, err := readTemplateContent(router, mainTmplName)
				if err != nil {
					return err
				}
				if err := writeTemplate(mainGoPath, tmpl, projConfig); err != nil {
					return err
				}
			}
		} else {
			if mappedName == "go.mod" {
				continue
			}
			
			tmpl, err := readTemplateContent(router, mappedName)
			if err == nil {
				if err := writeTemplate(fullPath, tmpl, projConfig); err != nil {
					return err
				}
			} else {
				f, err := os.Create(fullPath)
				if err != nil {
					return fmt.Errorf("failed to create file %s: %w", fullPath, err)
				}
				f.Close()
			}
		}
	}

	for dirPath := range createdDirs {
		cleanPath := filepath.Clean(dirPath)
		relPath, err := filepath.Rel(projectDir, cleanPath)
		if err != nil {
			continue
		}
		
		parts := strings.Split(filepath.ToSlash(relPath), "/")
		isGoPackageDir := false
		var pkgName string
		
		if len(parts) == 3 && parts[0] == "internal" && parts[1] == projectName {
			isGoPackageDir = true
			pkgName = parts[2]
		} else if len(parts) == 2 && parts[0] == "pkg" {
			isGoPackageDir = true
			pkgName = parts[1]
		}
		
		if isGoPackageDir && pkgName != "" {
			goFilePath := filepath.Join(cleanPath, pkgName+".go")
			content := fmt.Sprintf("package %s\n", pkgName)
			if err := os.WriteFile(goFilePath, []byte(content), 0644); err != nil {
				return fmt.Errorf("failed to create package file in %s: %w", cleanPath, err)
			}
		} else {
			entries, err := os.ReadDir(cleanPath)
			if err == nil && len(entries) == 0 {
				gitKeepPath := filepath.Join(cleanPath, ".gitkeep")
				_ = os.WriteFile(gitKeepPath, []byte(""), 0644)
			}
		}
	}

	fmt.Println("Initializing Go module...")
	cmdInit := exec.Command("go", "mod", "init", projectName)
	cmdInit.Dir = projectDir
	if out, err := cmdInit.CombinedOutput(); err != nil {
		return fmt.Errorf("go mod init failed: %s: %w", string(out), err)
	}

	cmdVer := exec.Command("go", "mod", "edit", "-go="+goVersion)
	cmdVer.Dir = projectDir
	_ = cmdVer.Run()

	if len(dependencies) > 0 {
		fmt.Println("Installing dependencies...")
		for _, dep := range dependencies {
			fmt.Printf("  go get %s...\n", dep)
			cmdGet := exec.Command("go", "get", dep)
			cmdGet.Dir = projectDir
			if out, err := cmdGet.CombinedOutput(); err != nil {
				fmt.Printf("Warning: failed to get %s: %s\n", dep, string(out))
			}
		}
	}

	fmt.Println("Tidying up Go module...")
	cmdTidy := exec.Command("go", "mod", "tidy")
	cmdTidy.Dir = projectDir
	_ = cmdTidy.Run()

	return nil
}

func detectProjectRoot() (root string, modulePath string, projectName string, err error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", "", "", err
	}

	for {
		goModPath := filepath.Join(dir, "go.mod")
		if _, err := os.Stat(goModPath); err == nil {
			content, err := os.ReadFile(goModPath)
			if err != nil {
				return "", "", "", fmt.Errorf("failed to read go.mod: %w", err)
			}
			
			lines := strings.Split(string(content), "\n")
			for _, line := range lines {
				trimmed := strings.TrimSpace(line)
				if strings.HasPrefix(trimmed, "module ") {
					modulePath = strings.TrimSpace(strings.TrimPrefix(trimmed, "module "))
					projectName = filepath.Base(modulePath)
					return dir, modulePath, projectName, nil
				}
			}
			return dir, "", filepath.Base(dir), nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", "", "", fmt.Errorf("go.mod not found in this directory or any parent directories")
}

func injectCodegenMarkers(projectRoot, projectName, modulePath, domainName, router string) error {
	var routerVar string
	switch router {
	case "gin":
		routerVar = "r"
	case "fiber":
		routerVar = "app"
	default:
		routerVar = "mux"
	}

	useDB := false
	if cfg, err := loadProjectConfig(projectRoot); err == nil {
		useDB = cfg.UseDB
	}

	importLines := []string{
		fmt.Sprintf("\t%s \"%s/internal/%s\"", domainName, modulePath, domainName),
	}

	var initArgs string
	if useDB {
		initArgs = "db"
	}

	wireLines := []string{
		fmt.Sprintf("\t// Wire up %s domain", domainName),
		fmt.Sprintf("\t%s.Init(%s).RegisterRoutes(%s)", domainName, initArgs, routerVar),
	}

	return injectMarkers(projectRoot, importLines, wireLines)
}

// injectHexModuleMarkers wires a hexagonal module into cmd/<project>/main.go,
// analogous to injectCodegenMarkers but pointing at modules/<name> and
// calling <name>.New(...).RegisterRoutes(...) instead of .Init(...).
func injectHexModuleMarkers(projectRoot, projectName, modulePath, moduleName, router string, useDB bool, messageBroker string) error {
	var routerVar string
	switch router {
	case "gin":
		routerVar = "r"
	case "fiber":
		routerVar = "app"
	default:
		routerVar = "mux"
	}

	importLines := []string{
		fmt.Sprintf("\t%s \"%s/modules/%s/module\"", moduleName, modulePath, moduleName),
	}

	var callArgs []string
	if useDB {
		callArgs = append(callArgs, "db")
	}
	if messageBroker != "direct" {
		callArgs = append(callArgs, "msgBus")
	}

	wireLines := []string{
		fmt.Sprintf("\t// Wire up %s module", moduleName),
		fmt.Sprintf("\t%s.New(%s).RegisterRoutes(%s)", moduleName, strings.Join(callArgs, ", "), routerVar),
	}

	return injectMarkers(projectRoot, importLines, wireLines)
}

// injectMarkers walks projectRoot's .go files looking for the
// `// [justgo:imports]` / `// [justgo:wire]` comment markers and splices
// importLines/wireLines directly after each, shared by both the standard
// (injectCodegenMarkers) and hexagonal (injectHexModuleMarkers) wiring paths.
func injectMarkers(projectRoot string, importLines, wireLines []string) error {
	return filepath.Walk(projectRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			name := info.Name()
			if name == ".git" || name == "vendor" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}

		if filepath.Ext(path) != ".go" {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		strContent := string(content)
		if !strings.Contains(strContent, "// [justgo:imports]") && !strings.Contains(strContent, "// [justgo:wire]") {
			return nil
		}

		lines := strings.Split(strContent, "\n")
		var newLines []string
		modified := false

		for _, line := range lines {
			if strings.Contains(line, "// [justgo:imports]") {
				newLines = append(newLines, line)
				for _, imp := range importLines {
					newLines = append(newLines, imp)
				}
				modified = true
			} else if strings.Contains(line, "// [justgo:wire]") {
				newLines = append(newLines, line)
				for _, w := range wireLines {
					newLines = append(newLines, w)
				}
				modified = true
			} else {
				newLines = append(newLines, line)
			}
		}

		if modified {
			output := strings.Join(newLines, "\n")
			err = os.WriteFile(path, []byte(output), 0644)
			if err != nil {
				return fmt.Errorf("failed to write modified file %s: %w", path, err)
			}
			fmt.Printf("Injected wire-up code into %s\n", filepath.Base(path))
		}

		return nil
	})
}

func parseLine(line string) (name string, isDir bool, level int, valid bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return "", false, 0, false
	}
	
	cleanLine := strings.Map(func(r rune) rune {
		if r == '│' || r == '├' || r == '└' || r == '─' || r == '|' || r == ' ' {
			return -1
		}
		return r
	}, line)
	
	if cleanLine == "" || strings.HasPrefix(cleanLine, "#") || cleanLine == "..." {
		return "", false, 0, false
	}

	if trimmed == "." {
		return ".", true, 0, true
	}

	runes := []rune(line)
	connIndex := -1
	for i, r := range runes {
		if r == '├' || r == '└' || r == '+' {
			connIndex = i
			break
		}
	}

	if connIndex == -1 {
		return "", false, 0, false
	}

	level = connIndex/4 + 1

	if connIndex+4 >= len(runes) {
		return "", false, 0, false
	}
	nameRunes := runes[connIndex+4:]
	name = strings.TrimSpace(string(nameRunes))
	
	if strings.HasSuffix(name, "/") {
		isDir = true
		name = strings.TrimSuffix(name, "/")
	}

	return name, isDir, level, true
}

func mapName(name string, projectName string) (string, bool) {
	name = strings.TrimSpace(name)
	if name == "..." {
		return "", false
	}
	
	if strings.HasPrefix(name, "your_app_1") {
		return strings.Replace(name, "your_app_1", projectName, 1), true
	}
	if strings.HasPrefix(name, "your_app_2") || strings.HasPrefix(name, "your_app_3") {
		return "", false
	}
	
	if strings.HasPrefix(name, "your_app_domain_1") {
		return strings.Replace(name, "your_app_domain_1", projectName, 1), true
	}
	if strings.HasPrefix(name, "your_app_domain_2") {
		return "", false
	}
	
	if strings.HasPrefix(name, "your_public_lib_1") {
		return strings.Replace(name, "your_public_lib_1", "utils", 1), true
	}
	if strings.HasPrefix(name, "your_public_lib_2") || strings.HasPrefix(name, "your_public_lib_3") {
		return "", false
	}
	
	return name, true
}

func writeTemplate(path, tmplContent string, config any) error {
	t, err := template.New("tmpl").Parse(tmplContent)
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %w", path, err)
	}
	defer f.Close()

	if err := t.Execute(f, config); err != nil {
		return fmt.Errorf("failed to execute template on %s: %w", path, err)
	}

	return nil
}

func toCamelCase(s string) string {
	if s == "" {
		return ""
	}
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == '-' || r == '_'
	})
	for i, part := range parts {
		if len(part) > 0 {
			parts[i] = strings.ToUpper(part[:1]) + part[1:]
		}
	}
	return strings.Join(parts, "")
}

func runGranularGenFlow(layer string, domainName string, extraArgs []string) {
	layer = strings.ToLower(strings.TrimSpace(layer))
	domainName = strings.ToLower(strings.TrimSpace(domainName))
	if domainName == "" {
		fmt.Println("Error: invalid domain name")
		return
	}

	root, modulePath, projectName, err := detectProjectRoot()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	router := "gin"
	useDB := false
	if cfg, err := loadProjectConfig(root); err == nil {
		router = cfg.Router
		useDB = cfg.UseDB
	}

	domainCamel := toCamelCase(domainName)
	domainLower := strings.ToLower(domainName)

	dcfg := DomainConfig{
		ProjectName: projectName,
		ModulePath:  modulePath,
		DomainName:  domainName,
		DomainCamel: domainCamel,
		DomainLower: domainLower,
		UseDB:       useDB,
	}

	// Define specific target files
	files := map[string]string{
		"model":      fmt.Sprintf("internal/%s/model/%s_model.go", domainLower, domainLower),
		"repository": fmt.Sprintf("internal/%s/repository/%s_repository.go", domainLower, domainLower),
		"usecase":    fmt.Sprintf("internal/%s/usecase/%s_usecase.go", domainLower, domainLower),
		"handler":    fmt.Sprintf("internal/%s/handler/%s_handler.go", domainLower, domainLower),
		"init":       fmt.Sprintf("internal/%s/init.go", domainLower),
		"routes":     fmt.Sprintf("internal/%s/routes.go", domainLower),
	}

	targetFilePath, exists := files[layer]
	if !exists {
		fmt.Printf("Error: unknown layer '%s'. Supported layers: model, repository, usecase, handler, init, routes\n", layer)
		return
	}

	// 1. If we are generating/updating a handler with an endpoint action:
	if layer == "handler" && len(extraArgs) > 0 {
		actionName := extraArgs[0]
		if strings.HasPrefix(actionName, "-") {
			fmt.Println("Error: please specify an endpoint action name. Example: justgo gen handler billing Create")
			return
		}
		appendHandlerEndpoint(root, router, projectName, modulePath, domainLower, domainCamel, actionName, extraArgs[1:])
		return
	}

	// 2. Otherwise, generate only the single specific layer file:
	dir := filepath.Dir(targetFilePath)
	if err := os.MkdirAll(filepath.Join(root, dir), 0755); err != nil {
		fmt.Printf("Error creating dir %s: %v\n", dir, err)
		return
	}

	tmpl, err := readTemplateContent(router, layer)
	if err != nil {
		fmt.Printf("Error reading template %s: %v\n", layer, err)
		return
	}

	fullPath := filepath.Join(root, targetFilePath)
	f, err := os.Create(fullPath)
	if err != nil {
		fmt.Printf("Error creating file %s: %v\n", targetFilePath, err)
		return
	}
	defer f.Close()

	t, err := template.New("tmpl").Parse(tmpl)
	if err == nil {
		_ = t.Execute(f, dcfg)
	}

	fmt.Printf("Generated specific layer '%s' at: %s\n", layer, targetFilePath)

	if layer == "repository" || layer == "usecase" {
		runMockgen(root, modulePath, domainLower, domainCamel)
	}

	// Inject wiring in main.go if we generated routes or init
	if layer == "routes" || layer == "init" {
		fmt.Println("Wiring up routes and imports in main.go...")
		_ = injectCodegenMarkers(root, projectName, modulePath, domainName, router)
	}
}

func appendHandlerEndpoint(root, router, projectName, modulePath, domainLower, domainCamel, actionName string, args []string) {
	// Parse method and path
	method := "GET"
	actionLower := strings.ToLower(actionName)
	actionCamel := toCamelCase(actionName)
	path := "/" + domainLower + "/" + actionLower

	for _, arg := range args {
		if strings.HasPrefix(arg, "--method=") {
			method = strings.ToUpper(strings.TrimPrefix(arg, "--method="))
		} else if strings.HasPrefix(arg, "--path=") {
			path = strings.TrimPrefix(arg, "--path=")
		}
	}

	handlerFile := filepath.Join(root, "internal", domainLower, "handler", domainLower+"_handler.go")
	routesFile := filepath.Join(root, "internal", domainLower, "routes.go")

	dcfg := DomainConfig{
		ProjectName: projectName,
		ModulePath:  modulePath,
		DomainName:  domainLower,
		DomainCamel: domainCamel,
		DomainLower: domainLower,
	}

	// 1. Generate base files first if they do not exist
	if _, err := os.Stat(handlerFile); os.IsNotExist(err) {
		fmt.Printf("Handler file not found. Scaffolding base handler first...\n")
		_ = os.MkdirAll(filepath.Dir(handlerFile), 0755)
		tmpl, _ := readTemplateContent(router, "handler")
		f, _ := os.Create(handlerFile)
		t, _ := template.New("tmpl").Parse(tmpl)
		_ = t.Execute(f, dcfg)
		f.Close()
	}
	if _, err := os.Stat(routesFile); os.IsNotExist(err) {
		fmt.Printf("Routes file not found. Scaffolding base routes first...\n")
		_ = os.MkdirAll(filepath.Dir(routesFile), 0755)
		tmpl, _ := readTemplateContent(router, "routes")
		f, _ := os.Create(routesFile)
		t, _ := template.New("tmpl").Parse(tmpl)
		_ = t.Execute(f, dcfg)
		f.Close()
	}

	// 2. Append new handler method to the handler file
	handlerContent, err := os.ReadFile(handlerFile)
	if err != nil {
		fmt.Printf("Error reading handler file: %v\n", err)
		return
	}

	var methodBlock string
	switch router {
	case "gin":
		methodBlock = fmt.Sprintf("\nfunc (h *%sHandler) %s(c *gin.Context) {\n\tctx := c.Request.Context()\n\t// TODO: implement endpoint business logic\n\tc.JSON(http.StatusOK, gin.H{\"message\": \"implemented %s\"})\n}\n", domainCamel, actionCamel, actionCamel)
	case "fiber":
		methodBlock = fmt.Sprintf("\nfunc (h *%sHandler) %s(c fiber.Ctx) error {\n\tctx := c.Context()\n\t// TODO: implement endpoint business logic\n\treturn c.JSON(fiber.Map{\"message\": \"implemented %s\"})\n}\n", domainCamel, actionCamel, actionCamel)
	default: // std
		methodBlock = fmt.Sprintf("\nfunc (h *%sHandler) %s(w http.ResponseWriter, r *http.Request) {\n\tctx := r.Context()\n\t// TODO: implement endpoint business logic\n\tw.Header().Set(\"Content-Type\", \"application/json\")\n\tw.Write([]byte(`{\"message\": \"implemented %s\"}`))\n}\n", domainCamel, actionCamel, actionCamel)
	}

	newHandlerContent := string(handlerContent) + methodBlock
	if err := os.WriteFile(handlerFile, []byte(newHandlerContent), 0644); err != nil {
		fmt.Printf("Error writing handler endpoint method: %v\n", err)
		return
	}
	fmt.Printf("Appended method %s to: %s\n", actionCamel, handlerFile)

	// 3. Register route in routes.go using comment marker
	routesContent, err := os.ReadFile(routesFile)
	if err != nil {
		fmt.Printf("Error reading routes file: %v\n", err)
		return
	}

	var routeLine string
	switch router {
	case "gin":
		methodCall := strings.ToUpper(method)
		routeLine = fmt.Sprintf("\tr.%s(\"%s\", d.Handler.%s)\n\t// [justgo:routes]", methodCall, path, actionCamel)
	case "fiber":
		var methodCall string
		switch strings.ToUpper(method) {
		case "POST":
			methodCall = "Post"
		case "PUT":
			methodCall = "Put"
		case "DELETE":
			methodCall = "Delete"
		default:
			methodCall = "Get"
		}
		routeLine = fmt.Sprintf("\tapp.%s(\"%s\", d.Handler.%s)\n\t// [justgo:routes]", methodCall, path, actionCamel)
	default: // std
		methodCall := strings.ToUpper(method)
		routeLine = fmt.Sprintf("\tmux.HandleFunc(\"%s %s\", d.Handler.%s)\n\t// [justgo:routes]", methodCall, path, actionCamel)
	}

	routesStr := string(routesContent)
	if strings.Contains(routesStr, "// [justgo:routes]") {
		routesStr = strings.Replace(routesStr, "// [justgo:routes]", routeLine, 1)
	} else {
		lastBrace := strings.LastIndex(routesStr, "}")
		if lastBrace != -1 {
			routesStr = routesStr[:lastBrace] + routeLine + "\n" + routesStr[lastBrace:]
		}
	}

	if err := os.WriteFile(routesFile, []byte(routesStr), 0644); err != nil {
		fmt.Printf("Error writing route registration: %v\n", err)
		return
	}
	fmt.Printf("Registered route %s %s in: %s\n", method, path, routesFile)
}

func runMockgen(projectRoot, modulePath, domainLower, domainCamel string) {
	mockgenPath, err := exec.LookPath("mockgen")
	if err != nil {
		// mockgen is not in PATH, skip silently
		return
	}

	mocksDir := filepath.Join(projectRoot, "internal", domainLower, "mocks")
	if err := os.MkdirAll(mocksDir, 0755); err != nil {
		fmt.Printf("Warning: failed to create mocks directory: %v\n", err)
		return
	}

	// 1. Generate repository mock
	repoSource := filepath.Join(projectRoot, "internal", domainLower, "repository", domainLower+"_repository.go")
	if _, err := os.Stat(repoSource); err == nil {
		repoDest := filepath.Join(mocksDir, "mock_"+domainLower+"_repository.go")
		cmd := exec.Command(mockgenPath,
			"-source="+repoSource,
			"-destination="+repoDest,
			"-package=mocks",
		)
		cmd.Dir = projectRoot
		if out, err := cmd.CombinedOutput(); err != nil {
			fmt.Printf("Warning: mockgen repository failed: %s\n", string(out))
		} else {
			fmt.Printf("Generated mock: %s\n", filepath.Join("internal", domainLower, "mocks", "mock_"+domainLower+"_repository.go"))
		}
	}

	// 2. Generate usecase mock
	usecaseSource := filepath.Join(projectRoot, "internal", domainLower, "usecase", domainLower+"_usecase.go")
	if _, err := os.Stat(usecaseSource); err == nil {
		usecaseDest := filepath.Join(mocksDir, "mock_"+domainLower+"_usecase.go")
		cmd := exec.Command(mockgenPath,
			"-source="+usecaseSource,
			"-destination="+usecaseDest,
			"-package=mocks",
		)
		cmd.Dir = projectRoot
		if out, err := cmd.CombinedOutput(); err != nil {
			fmt.Printf("Warning: mockgen usecase failed: %s\n", string(out))
		} else {
			fmt.Printf("Generated mock: %s\n", filepath.Join("internal", domainLower, "mocks", "mock_"+domainLower+"_usecase.go"))
		}
	}
}
