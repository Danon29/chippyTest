package main

import (
	"log"
	"net/http"
)

func main() {

	mux := http.NewServeMux()

	customHandler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(200)
		_, err := w.Write([]byte("OK"))
		if err != nil {
			return
		}
	}

	mux.HandleFunc("/healthz", customHandler)

	mux.Handle("/app/", http.StripPrefix("/app/", http.FileServer(http.Dir("."))))

	server := http.Server{Addr: ":8080", Handler: mux}

	log.Fatal(server.ListenAndServe())
}
