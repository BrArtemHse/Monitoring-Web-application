package main

import (
	"fmt"
	"log"
	"net/http"
)

func helloHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "Hello World!")
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "OK")
}

func main() {
	http.HandleFunc("/", helloHandler)
	http.HandleFunc("/health", healthHandler)

	addr := ":8080"
	log.Printf("App listening on %s\n", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("app failed: %v", err)
	}
}
