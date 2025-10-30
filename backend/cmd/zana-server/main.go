package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"zana-speech-backend/internal/config"
	"zana-speech-backend/internal/server"
)

func main() {
	cfg := config.Load()
	s, err := server.NewServer(cfg)
	if err != nil {
		log.Fatalf("failed to create server: %v", err)
		os.Exit(1)
	}
	addr := ":" + cfg.Port
	fmt.Printf("GITTER server listening on %s\n", addr)
	log.Fatal(http.ListenAndServe(addr, s.Router()))
}
