package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"time"

	"github.com/ddrowsy/family-friend-release/internal/service"
	"github.com/ddrowsy/family-friend-release/internal/service/database"
)

const readHeaderTimeout = 5 * time.Second

func main() {
	var listenAddress string
	var databasePath string
	flag.StringVar(
		&listenAddress,
		"listen",
		":8080",
		"HTTP listen address",
	)
	flag.StringVar(
		&databasePath,
		"db",
		"family-friend.db",
		"SQLite database path",
	)
	flag.Parse()

	db, err := database.Open(context.Background(), databasePath)
	if err != nil {
		log.Fatalf("open service database: %v", err)
	}

	server := &http.Server{
		Addr:              listenAddress,
		Handler:           service.NewHTTPHandler(service.NewStore(db)),
		ReadHeaderTimeout: readHeaderTimeout,
	}
	log.Printf("Family Friend control service listening on %s", listenAddress)

	serveErr := server.ListenAndServe()
	if closeErr := db.Close(); closeErr != nil {
		log.Printf("close service database: %v", closeErr)
	}
	if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
		log.Fatalf("serve HTTP: %v", serveErr)
	}
}
