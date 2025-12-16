package main

import (
	"fmt"
	"log"
	"net/http"
	"sync/atomic"
)

type apiConfig struct {
	fileserverHits atomic.Int32
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

func (cfg *apiConfig) hitHandler(w http.ResponseWriter, _ *http.Request) {
	hits := cfg.fileserverHits.Load()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(200)

	_, err := fmt.Fprintf(w, "Hits: %d", hits)
	if err != nil {
		return
	}
}

func (cfg *apiConfig) resetHandler(_ http.ResponseWriter, _ *http.Request) {
	cfg.fileserverHits.Store(0)
}

func main() {
	mux := http.NewServeMux()

	apiCfg := apiConfig{}

	customHandler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(200)
		_, err := w.Write([]byte("OK"))
		if err != nil {
			return
		}
	}

	mux.HandleFunc("GET /api/healthz", customHandler)

	mux.Handle("/app/",
		apiCfg.middlewareMetricsInc(
			http.StripPrefix("/app/", http.FileServer(http.Dir("."))),
		),
	)

	mux.HandleFunc("GET /api/metrics", apiCfg.hitHandler)
	mux.HandleFunc("POST /api/reset", apiCfg.resetHandler)

	server := http.Server{Addr: ":8080", Handler: mux}
	log.Fatal(server.ListenAndServe())
}
