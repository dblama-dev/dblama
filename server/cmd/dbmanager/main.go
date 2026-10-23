// Command dbmanager runs the db-manager HTTP server.
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

	"github.com/hitslab/db-manager/server"
	"github.com/hitslab/db-manager/server/internal/api"
	"github.com/hitslab/db-manager/server/internal/config"
	"github.com/hitslab/db-manager/server/internal/driver"
	"github.com/hitslab/db-manager/server/internal/driver/mysql"
	"github.com/hitslab/db-manager/server/internal/session"
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

	store := session.NewStore(cfg.SessionTTL, 1000, time.Now)
	stopJanitor := store.StartJanitor(time.Minute)
	defer stopJanitor()

	webFS, err := fs.Sub(server.WebFS, "web")
	if err != nil {
		return err
	}

	handler := api.NewRouter(cfg, store, registry, webFS)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv := &http.Server{Addr: cfg.Listen, Handler: handler}

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
