package main

// Bancada: a local workbench for LLM experiments.
//
// main.go starts everything:
//  1. moves into the project folder (where bancada.yaml lives),
//  2. loads API keys from .env and the config from bancada.yaml,
//  3. opens the SQLite database,
//  4. starts the Python evaluation service (if it is set up),
//  5. serves the API + web interface on http://127.0.0.1:<port> and opens the browser.

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"time"
)

// The built frontend (frontend/ builds into web/dist). The whole web/ folder is
// embedded so this compiles even before the frontend is built: web/README.md
// is always there. Without a build, the app shows a "not built yet" message.
//
//go:embed all:web
var webFiles embed.FS

func main() {
	dir := flag.String("dir", ".", "project folder (where bancada.yaml is)")
	noBrowser := flag.Bool("no-browser", false, "don't open the browser on startup")
	flag.Parse()

	if err := os.Chdir(*dir); err != nil {
		log.Fatalf("cannot open folder %s: %v", *dir, err)
	}
	loadEnvFile()

	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("bancada.yaml: %v", err)
	}
	for _, folder := range []string{cfg.Paths.Experiments, cfg.Paths.Evaluators, cfg.Paths.Datasets} {
		if err := os.MkdirAll(folder, 0o755); err != nil {
			log.Fatalf("cannot create folder %s: %v", folder, err)
		}
	}

	db, err := openDB(cfg.Paths.Database)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer db.Close()

	evaluatorProcess := startEvaluatorService(cfg.Server.EvaluatorURL)

	web, _ := fs.Sub(webFiles, "web/dist")
	app := &App{db: db, runner: newRunner(db), evals: newEvaluations(db), evalTests: newEvaluatorTests(db), web: web}

	// By default only listen on 127.0.0.1: the app (and your API keys) are not
	// reachable from other computers on the network. Inside Docker the app must
	// listen on 0.0.0.0 (BANCADA_HOST); docker-compose.yml still publishes the
	// port on 127.0.0.1 only.
	addr := fmt.Sprintf("%s:%d", envOr("BANCADA_HOST", "127.0.0.1"), cfg.Server.Port)
	server := &http.Server{Addr: addr, Handler: app.routes(), ReadHeaderTimeout: 10 * time.Second}

	go func() {
		log.Printf("Bancada running at http://localhost:%d", cfg.Server.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server: %v (is another Bancada already open on port %d?)", err, cfg.Server.Port)
		}
	}()
	if cfg.Server.OpenBrowser && !*noBrowser {
		openBrowser(fmt.Sprintf("http://localhost:%d", cfg.Server.Port))
	}

	// Wait for Ctrl+C, then shut down cleanly.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	log.Println("shutting down…")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	server.Shutdown(ctx)
	if evaluatorProcess != nil {
		evaluatorProcess.Process.Kill()
	}
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		log.Printf("open %s in your browser", url)
	}
}
