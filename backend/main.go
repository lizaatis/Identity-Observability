package main

import (
	"context"
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()

	r := gin.Default()
	r.SetTrustedProxies(nil) // do not trust any proxy; set to specific IPs if behind a reverse proxy
	r.GET("/api/v1/identities/:id", identityByID(pool))

	addr := os.Getenv("PORT")
	if addr == "" {
		addr = "8080"
	}
	log.Printf("listening on %s", addr)
	log.Fatal(r.Run(":" + addr))
}
