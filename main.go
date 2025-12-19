package main

import (
    "context"
    "database/sql"
    "encoding/json"
    "fmt"
    "log"
    "net/http"
    "os"
    "strings"
    "sync/atomic"
    "time"

    _ "github.com/lib/pq"
    "github.com/joho/godotenv"
    "github.com/google/uuid"

    "github.com/danon29/chippy/internal/database"
)

type apiConfig struct {
    fileserverHits atomic.Int32
    DB             *database.Queries
    platform       string
}

type User struct {
    ID        uuid.UUID `json:"id"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
    Email     string    `json:"email"`
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
    w.WriteHeader(http.StatusOK)

    _, err := fmt.Fprintf(w, "<html>\n  <body>\n    <h1>Welcome, Chirpy Admin</h1>\n    <p>Chirpy has been visited %d times!</p>\n  </body>\n</html>", hits)
    if err != nil {
        return
    }
}

func (cfg *apiConfig) resetHandler(w http.ResponseWriter, _ *http.Request) {
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    
    if cfg.platform != "dev" {
        http.Error(w, "Access denied", http.StatusForbidden)
        return
    }

    if err := cfg.DB.TruncateUsers(context.Background()); err != nil {
        http.Error(w, "Error while deleting all users: "+err.Error(), http.StatusInternalServerError)
        return
    }

    w.WriteHeader(http.StatusOK)
    fmt.Fprint(w, "✅ All users deleted successfully!")
}

func censor(body string, profane []string) string {
    lowered := strings.ToLower(body)
    result := body

    for _, bad := range profane {
        badLower := strings.ToLower(bad)

        for {
            idx := strings.Index(lowered, badLower)
            if idx == -1 {
                break
            }

            result = result[:idx] + "****" + result[idx+len(bad):]
            lowered = lowered[:idx] + "****" + lowered[idx+len(bad):]
        }
    }

    return result
}

func main() {
    if err := godotenv.Load(); err != nil {
        log.Fatal("Error loading .env file")
    }

    mux := http.NewServeMux()

    dbURL := os.Getenv("DB_URL")
    db, err := sql.Open("postgres", dbURL)
    if err != nil {
        log.Fatal("Error connecting to DB")
    }
    defer db.Close()

    dbQueries := database.New(db)

    cfg := apiConfig{
        DB:       dbQueries,
        platform: os.Getenv("PLATFORM"),
    }

    customHandler := func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "text/html; charset=utf-8")
        w.WriteHeader(http.StatusOK)
        _, err := w.Write([]byte("OK"))
        if err != nil {
            return
        }
    }

    postHandler := func(w http.ResponseWriter, r *http.Request) {
        type params struct {
            Body string `json:"body"`
        }

        profaneWords := []string{"kerfuffle", "sharbert", "fornax"}

        type errorResponse struct {
            Error string `json:"error"`
        }

        type validResponse struct {
            CleanedBody string `json:"cleaned_body"`
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

        cleaned := censor(p.Body, profaneWords)

        w.WriteHeader(http.StatusOK)
        json.NewEncoder(w).Encode(validResponse{CleanedBody: cleaned})
    }

    mux.Handle("/app/",
        cfg.middlewareMetricsInc(
            http.StripPrefix("/app/", http.FileServer(http.Dir("."))),
        ),
    )

    mux.HandleFunc("GET /admin/metrics", cfg.hitHandler)
	mux.HandleFunc("POST /admin/reset", cfg.resetHandler)

    mux.HandleFunc("GET /api/healthz", customHandler)
    mux.HandleFunc("POST /api/validate_chirp", postHandler)
    
    mux.HandleFunc("POST /api/users", func(w http.ResponseWriter, r *http.Request) {
        type params struct {
            Email string `json:"email"`
        }

        w.Header().Set("Content-Type", "application/json")
        
        var p params
        if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
            http.Error(w, "Invalid request", http.StatusBadRequest)
            return
        }

        user, err := cfg.DB.CreateUser(r.Context(), p.Email)
        if err != nil {
            http.Error(w, "Failed to create user", http.StatusInternalServerError)
            return
        }

		resultUser := User{
			ID: user.ID,
			CreatedAt: user.CreatedAt,
			UpdatedAt: user.UpdatedAt,
			Email: user.Email,
		}

        w.WriteHeader(http.StatusCreated)
        json.NewEncoder(w).Encode(resultUser)
    })

    server := http.Server{Addr: ":8080", Handler: mux}
    log.Fatal(server.ListenAndServe())
}
