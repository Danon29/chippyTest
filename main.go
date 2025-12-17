package main

import (
	"encoding/json"
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

	_, err := fmt.Fprintf(w, "<html>\n  <body>\n    <h1>Welcome, Chirpy Admin</h1>\n    <p>Chirpy has been visited %d times!</p>\n  </body>\n</html>", hits)
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

	postHandler := func(w http.ResponseWriter, r *http.Request) {
		type params struct {
			Body string `json:"body"`
		}

		type errorResponse struct {
			Error string `json:"error"`
		}

		type validResponse struct {
			Valid bool `json:"valid"`
		}

		w.Header().Set("Content-Type", "application/json")

		var p params
		decoder := json.NewDecoder(r.Body)
		err := decoder.Decode(&p)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(errorResponse{Error: "Something went wrong"})
			return
		}

		if len(p.Body) > 140 {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(errorResponse{Error: "Chirp is too long"})
			return
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(validResponse{Valid: true})
	}

	mux.HandleFunc("GET /api/healthz", customHandler)

	mux.Handle("/app/",
		apiCfg.middlewareMetricsInc(
			http.StripPrefix("/app/", http.FileServer(http.Dir("."))),
		),
	)

	mux.HandleFunc("GET /admin/metrics", apiCfg.hitHandler)
	mux.HandleFunc("POST /admin/reset", apiCfg.resetHandler)

	mux.HandleFunc("POST /api/validate_chirp", postHandler)

	server := http.Server{Addr: ":8080", Handler: mux}
	log.Fatal(server.ListenAndServe())
}
