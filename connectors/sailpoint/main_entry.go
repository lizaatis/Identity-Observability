package sailpoint

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
)

// Run is the main entry point for the SailPoint connector
func Run() {
	cfg := LoadConfig()

	connector, err := NewSailPointConnector(cfg)
	if err != nil {
		log.Fatalf("Failed to create connector: %v", err)
	}
	defer connector.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Shutting down...")
		cancel()
	}()

	// Run sync
	if err := connector.Sync(ctx); err != nil {
		log.Fatalf("Sync failed: %v", err)
	}

	log.Println("Sync completed successfully")
}
