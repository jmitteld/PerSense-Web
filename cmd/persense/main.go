// Per%Sense Web — financial services calculator
//
// This is the main entry point for the web application.
// It serves both the REST API and the HTMX frontend.
//
// Usage:
//
//	go run ./cmd/persense
//	# Open http://localhost:8080
package main

import (
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"

	"github.com/persense/persense-port/internal/api"
)

//go:embed static
var staticFiles embed.FS

func main() {
	port := flag.Int("port", 8080, "HTTP server port")
	flag.Parse()

	mux := http.NewServeMux()

	// API endpoints
	mux.HandleFunc("/api/mortgage/calc", api.HandleMortgageCalc)
	mux.HandleFunc("/api/amortization/calc", api.HandleAmortizationCalc)
	mux.HandleFunc("/api/presentvalue/calc", api.HandlePVCalc)

	// Health check
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})

	// Static files (HTMX frontend)
	staticSub, err := fs.Sub(staticFiles, "static")
	if err != nil {
		log.Fatal(err)
	}
	mux.Handle("/", http.FileServer(http.FS(staticSub)))

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("Per%%Sense Web starting on http://localhost%s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}
