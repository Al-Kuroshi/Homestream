package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"personaltv/internal/api"
	"personaltv/internal/channels"
	"personaltv/internal/db"
	"personaltv/internal/mediastore"
	"personaltv/internal/playback"
	"personaltv/internal/repository/sqlite"
	"personaltv/web"
)

func main() {
	dbPath := getEnv("PERSONALTV_DB_PATH", "personaltv.db")
	port := getEnv("PERSONALTV_PORT", "8080")

	conn, err := db.Open(dbPath)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}
	defer conn.Close()

	sourceRepo := sqlite.NewMediaSourceRepository(conn)
	itemRepo := sqlite.NewMediaItemRepository(conn)
	channelRepo := sqlite.NewChannelRepository(conn)
	slotRepo := sqlite.NewSlotRepository(conn)

	scanner := mediastore.NewScanner(sourceRepo, itemRepo)
	channelSvc := channels.NewService(channelRepo, slotRepo, itemRepo)

	sessionsDir := getEnv("PERSONALTV_SESSIONS_DIR", filepath.Join(os.TempDir(), "personaltv-playback"))
	if err := playback.CleanOrphanedSessions(sessionsDir); err != nil {
		log.Printf("warning: failed to clean orphaned playback sessions: %v", err)
	}
	sessions := playback.NewSessionManager(sessionsDir, 60*time.Second)
	playbackSvc := playback.NewService(channelSvc, sourceRepo, itemRepo, sessions)

	sweepCtx, stopSweep := context.WithCancel(context.Background())
	defer stopSweep()
	go sessions.Run(sweepCtx, 30*time.Second)

	server := api.NewServer(sourceRepo, itemRepo, scanner, channelSvc)
	server.SetPlaybackService(playbackSvc)

	webHandler, err := web.Handler()
	if err != nil {
		log.Fatalf("failed to load embedded frontend: %v", err)
	}
	server.SetStaticHandler(webHandler)

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: server.Routes(),
		// ReadHeaderTimeout and IdleTimeout bound how long a slow or idle
		// client can hold a connection. WriteTimeout is deliberately left
		// unset: playback streams long-lived responses (direct-play range
		// requests, HLS segments), and a write deadline would cut those
		// off mid-stream.
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// SIGINT/SIGTERM (what `docker stop` sends) must drain in-flight requests
	// instead of killing them mid-response.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	serverErr := make(chan error, 1)
	go func() {
		log.Printf("Personal TV listening on :%s (db: %s)", port, dbPath)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
		close(serverErr)
	}()

	select {
	case err := <-serverErr:
		if err != nil {
			log.Fatalf("server failed: %v", err)
		}
	case <-ctx.Done():
		stop() // restore default signal handling: a second signal aborts immediately
		log.Println("shutdown signal received, draining in-flight requests")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("graceful shutdown failed: %v", err)
		}
		stopSweep()
		sessions.Close()
		log.Println("Personal TV stopped")
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
