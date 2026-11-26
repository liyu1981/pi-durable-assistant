package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/liyu1981/pi-durable-assistant/internal/config"
	"github.com/liyu1981/pi-durable-assistant/internal/httpapi"
	"github.com/liyu1981/pi-durable-assistant/internal/runtime"
)

//go:embed web/dist
var webDist embed.FS

func newServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
}

func redirectToHTTPS(httpsPort int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if parsed, _, err := net.SplitHostPort(r.Host); err == nil {
			host = parsed
		}
		target := "https://" + host
		if httpsPort != 443 {
			target += ":" + strconv.Itoa(httpsPort)
		}
		http.Redirect(w, r, target+r.URL.RequestURI(), http.StatusPermanentRedirect)
	})
}

func main() {
	cfg := config.Load()
	if cfg.Production && cfg.Password == "" {
		fmt.Fprintln(os.Stderr, "DEMO_PASSWORD is required in production")
		os.Exit(1)
	}
	if cfg.HTTPMode != "http" && cfg.HTTPMode != "https" && cfg.HTTPMode != "both" {
		fmt.Fprintln(os.Stderr, "HTTP_MODE must be http, https, or both")
		os.Exit(1)
	}
	rt, err := runtime.Open(context.Background(), cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "startup failed:", cfg.Redact(err.Error()))
		os.Exit(1)
	}
	assets, err := fs.Sub(webDist, "web/dist")
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to load static assets:", err)
		os.Exit(1)
	}
	api, err := httpapi.NewServer(rt, assets)
	if err != nil {
		fmt.Fprintln(os.Stderr, "HTTP initialization failed:", err)
		os.Exit(1)
	}

	host := "127.0.0.1"
	port := cfg.Port
	if cfg.Dev {
		host = "0.0.0.0"
		port = 3111
	} else if cfg.Password != "" {
		host = "0.0.0.0"
	}
	addr := fmt.Sprintf("%s:%d", host, port)

	var plain *http.Server
	var secure *http.Server
	switch cfg.HTTPMode {
	case "http":
		plain = newServer(addr, api)
	case "https", "both":
		if _, err := os.Stat(cfg.CertFile); err != nil {
			fmt.Fprintf(os.Stderr, "TLS certificate not found at %s (generate with mkcert)\n", cfg.CertFile)
			os.Exit(1)
		}
		secureAddr := addr
		if cfg.HTTPMode == "both" {
			secureAddr = fmt.Sprintf("%s:%d", host, cfg.HTTPSPort)
			plain = newServer(addr, redirectToHTTPS(cfg.HTTPSPort))
		}
		secure = newServer(secureAddr, api)
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-stop
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if plain != nil {
			_ = plain.Shutdown(ctx)
		}
		if secure != nil {
			_ = secure.Shutdown(ctx)
		}
	}()

	if plain != nil {
		fmt.Printf("pi-durable-assistant started: http://%s, model %s, PID %d\n", plain.Addr, rt.Label(), os.Getpid())
	}
	if secure != nil {
		fmt.Printf("pi-durable-assistant started: https://%s, model %s, PID %d\n", secure.Addr, rt.Label(), os.Getpid())
	}

	if plain != nil && secure != nil {
		go func() {
			if err := plain.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				fmt.Fprintln(os.Stderr, "http server exited:", err)
			}
		}()
	}
	if secure != nil {
		if err := secure.ListenAndServeTLS(cfg.CertFile, cfg.KeyFile); err != nil && err != http.ErrServerClosed {
			fmt.Fprintln(os.Stderr, "https server exited:", err)
			os.Exit(1)
		}
		return
	}
	if err := plain.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Fprintln(os.Stderr, "server exited:", err)
		os.Exit(1)
	}
}
