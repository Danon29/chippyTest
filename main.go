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
    "github.com/danon29/chippy/internal/auth"
)

type apiConfig struct {
    fileserverHits atomic.Int32
    DB             *database.Queries
    platform       string
    jwtSecret      string
}

type User struct {
    ID        uuid.UUID `json:"id"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
    Email     string    `json:"email"`
    Token     string    `json:"token"`
}

type Chirp struct {
    ID        uuid.UUID `json:"id"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
    Body     string    `json:"body"`
    UserId  uuid.UUID `json:"user_id"`
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

    if err := cfg.DB.DeleteUsers(context.Background()); err != nil {
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
        jwtSecret: os.Getenv("JWT_SECRET"),
    }

    customHandler := func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "text/html; charset=utf-8")
        w.WriteHeader(http.StatusOK)
        _, err := w.Write([]byte("OK"))
        if err != nil {
            return
        }
    }


    mux.Handle("/app/",
        cfg.middlewareMetricsInc(
            http.StripPrefix("/app/", http.FileServer(http.Dir("."))),
        ),
    )

    mux.HandleFunc("GET /admin/metrics", cfg.hitHandler)
	mux.HandleFunc("POST /admin/reset", cfg.resetHandler)

    mux.HandleFunc("GET /api/healthz", customHandler)

    mux.HandleFunc("GET /api/chirps", func(w http.ResponseWriter, r *http.Request) {
        chirps, err := cfg.DB.GetChirps(r.Context())
        if err != nil {
            http.Error(w, "Error", http.StatusInternalServerError)
            return
        }

        resp := make([]Chirp, 0, len(chirps))

        for _, chirp := range chirps {
            resp = append(resp, Chirp{
                ID: chirp.ID,
                CreatedAt: chirp.CreatedAt,
                UpdatedAt: chirp.CreatedAt,
                Body: chirp.Body,
                UserId: chirp.UserID,
            })
        }
        w.WriteHeader(http.StatusOK)
        json.NewEncoder(w).Encode(resp)
    })
    mux.HandleFunc("GET /api/chirps/{chirpId}", func(w http.ResponseWriter, r *http.Request) {
        chirpIdStr := r.PathValue("chirpId")
        if chirpIdStr == "" {
            http.Error(w, "chirpId is required", http.StatusBadRequest)
            return
        }

        chirpID, err := uuid.Parse(chirpIdStr)
        if err != nil {
            http.Error(w, "invalid chirpId", http.StatusBadRequest)
            return
        }

        chirp, err := cfg.DB.GetChirp(r.Context(), chirpID)
        if err != nil {
            http.Error(w, "Name parameter is required", http.StatusNotFound)
		    return 
        }
        

        result := Chirp{
                ID: chirp.ID,
                CreatedAt: chirp.CreatedAt,
                UpdatedAt: chirp.CreatedAt,
                Body: chirp.Body,
                UserId: chirp.UserID,
            }
        w.WriteHeader(http.StatusOK)
        json.NewEncoder(w).Encode(result)
    })
    mux.HandleFunc("POST /api/chirps", func(w http.ResponseWriter, r *http.Request) {
        type params struct {
            Body string `json:"body"`
        }

        profaneWords := []string{"kerfuffle", "sharbert", "fornax"}

        type errorResponse struct {
            Error string `json:"error"`
        }

        type validResponse struct {
            Body string `json:"body"`
            UserId uuid.UUID `json:"user_id"`
        }

        token, err := auth.GetBearerToken(r.Header)
        if err != nil {
            http.Error(w, "No valid tokens", http.StatusUnauthorized)
            return
        }

        userID, err := auth.ValidateJWT(token, cfg.jwtSecret) 
        if err != nil || userID == uuid.Nil {
            http.Error(w, "Invalid token", http.StatusUnauthorized)
            return
        }

        w.Header().Set("Content-Type", "application/json")

        var p params
        decoder := json.NewDecoder(r.Body)
        err = decoder.Decode(&p)
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

        chirp, err := cfg.DB.CreateChirp(r.Context(), database.CreateChirpParams{
            Body: cleaned,
            UserID: userID,
        })

        if err != nil {
            w.WriteHeader(http.StatusBadRequest)
            json.NewEncoder(w).Encode(errorResponse{Error: "Error while creating chirp"})
            return
        }
        w.WriteHeader(http.StatusCreated)
        json.NewEncoder(w).Encode(Chirp{
                ID: chirp.ID,
                CreatedAt: chirp.CreatedAt,
                UpdatedAt: chirp.CreatedAt,
                Body: chirp.Body,
                UserId: chirp.UserID,
            })
    })
    
    mux.HandleFunc("POST /api/users", func(w http.ResponseWriter, r *http.Request) {
        type params struct {
            Email string `json:"email"`
            Password string `json:"password"`
        }

        w.Header().Set("Content-Type", "application/json")
        
        var p params
        if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
            http.Error(w, "Invalid request", http.StatusBadRequest)
            return
        }

        hashedPassword, err := auth.HashPassword(p.Password)
        if err != nil {
            http.Error(w, "Failed to create user", http.StatusInternalServerError)
            return
        }

        user, err := cfg.DB.CreateUser(r.Context(), database.CreateUserParams{
            Email: p.Email, HashedPassword: hashedPassword,
        })
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

    mux.HandleFunc("POST /api/login", func(w http.ResponseWriter, r *http.Request) {
        type params struct {
            Password string `json:"password"`
            Email string `json:"email"`
            ExpiresInSeconds int `json:"expires_in_seconds"`
        }

        var p params
        if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
            http.Error(w, "Invalid request", http.StatusBadRequest)
            return
        }

        user, err := cfg.DB.FindUser(r.Context(), p.Email)
       

        isValidPassword, err := auth.CheckPasswordHash(p.Password, user.HashedPassword)
        if err != nil {
            http.Error(w, "Unknown error", http.StatusInternalServerError)
            return
        }

        if err != nil || !isValidPassword {
            w.Header().Set("Content-Type", "application/json")
            w.WriteHeader(http.StatusUnauthorized)
            json.NewEncoder(w).Encode(struct{ Error string }{"Incorrect email or password"})
            return
        }

        var expiresTime time.Duration
        if p.ExpiresInSeconds == 0 || p.ExpiresInSeconds > 3600 {
            expiresTime = 3600 * time.Second 
        } else {
            expiresTime = time.Duration(p.ExpiresInSeconds) * time.Second
        }

        token, err := auth.MakeJWT(user.ID, cfg.jwtSecret, expiresTime)
        if err != nil {
            http.Error(w, "Unknown error", http.StatusInternalServerError)
            return
        }

        resultUser := User{
			ID: user.ID,
			CreatedAt: user.CreatedAt,
			UpdatedAt: user.UpdatedAt,
			Email: user.Email,
            Token: token,
		}

        w.WriteHeader(http.StatusOK)
        json.NewEncoder(w).Encode(resultUser)
    })

    server := http.Server{Addr: ":8080", Handler: mux}
    log.Fatal(server.ListenAndServe())
}
