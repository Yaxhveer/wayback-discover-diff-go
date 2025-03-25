package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"wayback-discover-diff-go/internal/handlers"
)

func main() {
	redisOpts, err := redis.ParseURL("redis://localhost:6379/5")
	if err != nil {
		log.Fatalf("Failed to parse Redis URL: %v", err)
	}
	redisClient := redis.NewClient(redisOpts)

	router := gin.Default()
	diffHandler := handlers.NewHandler(redisClient)
	router.GET("/", diffHandler.Root)
	router.GET("/simhash", diffHandler.GetSimhash)
	router.GET("/calculate-simhash", diffHandler.CalculateSimhash)
	router.GET("/job", diffHandler.GetJobStatus)

	// Create an HTTP server with the Gin router.
	srv := &http.Server{
		Addr:    ":8080",
		Handler: router,
	}

	// Start the server in a goroutine.
	go func() {
		log.Println("Server is running on :8080")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s", err)
		}
	}()

	// Create a channel to listen for OS interrupt signals.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit // Block until an interrupt signal is received.
	log.Println("Shutdown signal received, shutting down server...")

	// Create a context with a timeout for graceful shutdown.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Attempt graceful shutdown.
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}
	log.Println("Server exiting")
}
