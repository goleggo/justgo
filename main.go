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

//go:embed templates/*
var templatesFS embed.FS

type JustGoConfig struct {
	ProjectName string `json:"projectName"`
	Router      string `json:"router"`
	UseDB       bool   `json:"useDB"`
	DBEngine    string `json:"dbEngine"`
	UseObs      bool   `json:"useObs"`
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
	ProjectName string
	ModulePath  string
	Router      string
	UseDB       bool
	DBEngine    string
	UseObs      bool
}

type DomainConfig struct {
	ProjectName string
	ModulePath  string
	DomainName  string
	DomainCamel string
	DomainLower string
	UseDB       bool
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
		if len(os.Args) == 3 {
			runGenModuleFlow(os.Args[2])
		} else {
			runGranularGenFlow(os.Args[2], os.Args[3], os.Args[4:])
		}
	case "agents":
		runAgentsFlow()
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
	fmt.Println("  agents                      Generate AI agent/harness instructions (AGENTS.md, Claude Skill, Kiro steering)")
	fmt.Println("\nSupported Layers:")
	fmt.Println("  model, repository, usecase, handler, init, routes")
	fmt.Println("\nHandler Append Options:")
	fmt.Println("  --method=<METHOD>           HTTP Method (e.g. GET, POST, PUT, DELETE) [default: GET]")
	fmt.Println("  --path=<PATH>               Route path [default: /<domain>/<action>]")
	fmt.Println("\nExamples:")
	fmt.Println("  justgo new")
	fmt.Println("  justgo gen billing")
	fmt.Println("  justgo gen model billing")
	fmt.Println("  justgo gen handler billing Create --method=POST --path=/api/v1/billing")
	fmt.Println("  justgo agents")
	fmt.Println("\nNote: Mocks are automatically generated using mockgen (run 'make mock' to regenerate).")
	fmt.Println("==========================================================")
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

	// 5. Prompt Dependencies
	fmt.Print("Enter Dependencies (space-separated, e.g. github.com/joho/godotenv): ")
	inputDeps, _ := reader.ReadString('\n')
	depsStr := strings.TrimSpace(inputDeps)
	var dependencies []string
	if depsStr != "" {
		dependencies = strings.Fields(depsStr)
	}

	// Add default packages automatically
	dependencies = append(dependencies, "github.com/joho/godotenv")
	if router == "gin" {
		dependencies = append(dependencies, "github.com/gin-gonic/gin")
	} else if router == "fiber" {
		dependencies = append(dependencies, "github.com/gofiber/fiber/v3")
	}

	if useObs {
		dependencies = append(dependencies, "github.com/goleggo/observer@v0.1.4")
	}

	// Add database driver package
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

	// 6. Confirm Settings
	fmt.Println("\nConfiguration Summary:")
	fmt.Printf("  - Project Name : %s\n", projectName)
	fmt.Printf("  - Go Version   : %s\n", goVersion)
	fmt.Printf("  - HTTP Router  : %s\n", router)
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
	err := generateProject(projectName, goVersion, router, useDB, dbEngine, useObs, dependencies)
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

func generateProject(projectName, goVersion, router string, useDB bool, dbEngine string, useObs bool, dependencies []string) error {
	projectDir := filepath.Join(".", projectName)
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		return fmt.Errorf("failed to create root dir: %w", err)
	}

	// Save config
	cfg := JustGoConfig{
		ProjectName: projectName,
		Router:      router,
		UseDB:       useDB,
		DBEngine:    dbEngine,
		UseObs:      useObs,
	}
	cfgBytes, err := json.MarshalIndent(cfg, "", "  ")
	if err == nil {
		_ = os.WriteFile(filepath.Join(projectDir, ".justgo.json"), cfgBytes, 0644)
	}

	projConfig := ProjectConfig{
		ProjectName: projectName,
		UseDB:       useDB,
		DBEngine:    dbEngine,
		UseObs:      useObs,
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

		// Create db/, db/queries/, and db/migrations/ directories
		dbSchemaDir := filepath.Join(projectDir, "db")
		dbQueriesDir := filepath.Join(projectDir, "db", "queries")
		dbMigrationsDir := filepath.Join(projectDir, "db", "migrations")
		if err := os.MkdirAll(dbQueriesDir, 0755); err != nil {
			return fmt.Errorf("failed to create db queries dir: %w", err)
		}
		if err := os.MkdirAll(dbMigrationsDir, 0755); err != nil {
			return fmt.Errorf("failed to create db migrations dir: %w", err)
		}
		// Create a placeholder .gitkeep in migrations
		_ = os.WriteFile(filepath.Join(dbMigrationsDir, ".gitkeep"), []byte(""), 0644)

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

	lines := strings.Split(defaultStructure, "\n")
	
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
				tmpl, err := readTemplateContent(router, "main.go")
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

	err := filepath.Walk(projectRoot, func(path string, info os.FileInfo, err error) error {
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

	return err
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

func writeTemplate(path, tmplContent string, config ProjectConfig) error {
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
