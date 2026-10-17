package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Ritik-Sharma2311/device-registry/internal/api"
	"github.com/Ritik-Sharma2311/device-registry/internal/auth"
	"github.com/Ritik-Sharma2311/device-registry/internal/device"
	"github.com/Ritik-Sharma2311/device-registry/internal/events"
	"github.com/Ritik-Sharma2311/device-registry/internal/store/memory"
	"github.com/Ritik-Sharma2311/device-registry/internal/store/mongodb"
	"github.com/Ritik-Sharma2311/device-registry/internal/store/postgres"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	backend := flag.String("store", "memory", "storage backend: memory, postgres or mongodb")
	natsURL := flag.String("nats", "", "NATS URL; empty disables event publishing")
	flag.Parse()

	// Secrets come from the environment, never from flags (they show up in ps).
	users, err := auth.ParseUsers(os.Getenv("REGISTRY_USERS"))
	if err != nil || len(users) == 0 {
		log.Fatal("set REGISTRY_USERS=name:password:role[,name:password:role] (roles: admin, viewer)")
	}
	authn, err := auth.New(os.Getenv("JWT_SECRET"), 15*time.Minute, users)
	if err != nil {
		log.Fatalf("JWT_SECRET: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	store, err := openStore(ctx, *backend)
	cancel()
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer store.Close()

	var pub device.Publisher
	if *natsURL != "" {
		n, err := events.Connect(*natsURL)
		if err != nil {
			log.Fatalf("nats: %v", err)
		}
		defer n.Close()
		pub = n
	}

	srv := &http.Server{
		Addr:              *addr,
		Handler:           api.New(device.NewService(store, pub), authn).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		log.Printf("device-registry on %s (store=%s nats=%q)", *addr, *backend, *natsURL)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	sctx, scancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer scancel()
	_ = srv.Shutdown(sctx)
}

func openStore(ctx context.Context, backend string) (device.Store, error) {
	switch backend {
	case "memory":
		return memory.New(), nil
	case "postgres":
		return postgres.Open(ctx, os.Getenv("POSTGRES_DSN"))
	case "mongodb":
		return mongodb.Open(ctx, os.Getenv("MONGO_URI"), "registry")
	default:
		return nil, errors.New("unknown -store " + backend)
	}
}
