// Command dblama runs the HTTP server.
package main

import (
	"context"
	"errors"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dblama-dev/dblama"
	"github.com/dblama-dev/dblama/internal/api"
	"github.com/dblama-dev/dblama/internal/config"
	"github.com/dblama-dev/dblama/internal/driver"
	"github.com/dblama-dev/dblama/internal/driver/mysql"
	"github.com/dblama-dev/dblama/internal/driver/postgres"
	"github.com/dblama-dev/dblama/internal/session"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}

	// MySQL and MariaDB share one connector; the meta ID is what the
	// login screen offers and both default to port 3306.
	registry := driver.NewRegistry()
	registry.Register(driver.DriverMeta{ID: "mysql", Name: "MySQL", DefaultPort: "3306"}, mysql.Connect)
	registry.Register(driver.DriverMeta{ID: "mariadb", Name: "MariaDB", DefaultPort: "3306"}, mysql.Connect)
	registry.Register(driver.DriverMeta{ID: "postgres", Name: "PostgreSQL", DefaultPort: "5432"}, postgres.Connect)

	store := session.NewStore(cfg.SessionTTL, 1000, time.Now)
	stopJanitor := store.StartJanitor(time.Minute)
	defer stopJanitor()

	webFS, err := fs.Sub(dblama.WebFS, "frontend/dist")
	if err != nil {
		return err
	}

	handler := api.NewRouter(cfg, store, registry, webFS)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	ln, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		return err
	}
	log.Printf("listening on %s", ln.Addr())

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Serve(ln)
	}()

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		log.Println("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
	return nil
}
