package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
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
	webassets "github.com/liyu1981/pi-durable-assistant/web"
)

// version is reported by "pida version" and "--version".
var version = "0.1.0"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "pida:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		printUsage(os.Stdout)
		return nil
	}
	switch args[0] {
	case "serve":
		return runServe(args[1:])
	case "config":
		return runConfig(args[1:])
	case "service-install":
		return runServiceInstall(args[1:])
	case "service-uninstall":
		return runServiceUninstall(args[1:])
	case "version", "--version", "-V":
		fmt.Printf("pida %s\n", version)
		return nil
	case "help", "--help", "-h":
		if len(args) > 1 {
			switch args[1] {
			case "serve":
				return runServe([]string{"--help"})
			case "config":
				return runConfig([]string{"--help"})
			case "service-install":
				return runServiceInstall([]string{"--help"})
			case "service-uninstall":
				return runServiceUninstall([]string{"--help"})
			}
		}
		printUsage(os.Stdout)
		return nil
	default:
		printUsage(os.Stderr)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func printUsage(w io.Writer) {
	fmt.Fprint(w, `pida - durable, self-hosted personal assistant

Usage:
  pida <command> [options]

Commands:
  serve              Start the server (reads ~/.pida/pida.env by default)
  config             Write an example config file (pida.env)
  service-install    Install a systemd user service
  service-uninstall  Remove the systemd user service
  version            Print the version
  help               Show this help

Run "pida <command> --help" for command options.
`)
}

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

type serveOptions struct {
	configPath string
	port       int
	dataDir    string
	httpMode   string
	httpsPort  int
	certFile   string
	keyFile    string
	password   string
	dev        bool
	production bool
}

func newServeFlags(opts *serveOptions) *flag.FlagSet {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&opts.configPath, "config", "", "config file to read (default ~/.pida/pida.env)")
	fs.IntVar(&opts.port, "port", 0, "HTTP port (default 3000)")
	fs.StringVar(&opts.dataDir, "data-dir", "", "data directory (default ~/.pida)")
	fs.StringVar(&opts.httpMode, "http-mode", "", "http, https, or both")
	fs.IntVar(&opts.httpsPort, "https-port", 0, "HTTPS port (default 3443)")
	fs.StringVar(&opts.certFile, "cert-file", "", "TLS certificate file")
	fs.StringVar(&opts.keyFile, "key-file", "", "TLS key file")
	fs.StringVar(&opts.password, "password", "", "access password (prefer the config file)")
	fs.BoolVar(&opts.dev, "dev", false, "development mode: bind 0.0.0.0:3111 and force HTTPS")
	fs.BoolVar(&opts.production, "production", false, "production mode: require a password and bind 0.0.0.0")
	return fs
}

func printServeUsage(fs *flag.FlagSet) {
	fmt.Println(`Usage: pida serve [options]

Start the server. By default it reads ~/.pida/pida.env and stores data in
~/.pida. Command-line options win over the file and the process environment.

Options:`)
	fs.SetOutput(os.Stdout)
	fs.PrintDefaults()
	fs.SetOutput(io.Discard)
}

func runServe(args []string) error {
	var opts serveOptions
	fs := newServeFlags(&opts)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printServeUsage(fs)
			return nil
		}
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("serve: unexpected argument %q", fs.Arg(0))
	}
	set := make(map[string]bool, 10)
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })

	configPath := opts.configPath
	if configPath == "" {
		var err error
		if configPath, err = config.DefaultFile(); err != nil {
			return err
		}
	}
	values, err := config.LoadFile(configPath)
	if err != nil {
		return err
	}
	config.ApplyToEnv(values)

	cfg := config.Load()
	if set["port"] {
		cfg.Port = opts.port
	}
	if set["http-mode"] {
		cfg.HTTPMode = opts.httpMode
	}
	if set["https-port"] {
		cfg.HTTPSPort = opts.httpsPort
	}
	if set["cert-file"] {
		cfg.CertFile = opts.certFile
	}
	if set["key-file"] {
		cfg.KeyFile = opts.keyFile
	}
	if set["password"] {
		cfg.Password = opts.password
	}
	cfg.Dev = cfg.Dev || opts.dev
	cfg.Production = cfg.Production || opts.production

	if set["data-dir"] {
		cfg.DataDir = opts.dataDir
	} else if os.Getenv("DATA_DIR") == "" && !cfg.Dev {
		dir, err := config.DefaultDir()
		if err != nil {
			return err
		}
		cfg.DataDir = dir
	}
	return serve(cfg)
}

func serve(cfg config.Config) error {
	if cfg.Production && cfg.Password == "" {
		return errors.New("DEMO_PASSWORD is required in production")
	}
	if cfg.HTTPMode != "http" && cfg.HTTPMode != "https" && cfg.HTTPMode != "both" {
		return errors.New("HTTP_MODE must be http, https, or both")
	}
	rt, err := runtime.Open(context.Background(), cfg)
	if err != nil {
		return fmt.Errorf("startup failed: %s", cfg.Redact(err.Error()))
	}
	assets, err := fs.Sub(webassets.Dist, "dist")
	if err != nil {
		return fmt.Errorf("failed to load static assets: %w", err)
	}
	api, err := httpapi.NewServer(rt, assets)
	if err != nil {
		return fmt.Errorf("HTTP initialization failed: %w", err)
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
			return fmt.Errorf("TLS certificate not found at %s (generate with mkcert)", cfg.CertFile)
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
		fmt.Printf("pida started: http://%s, model %s, PID %d\n", plain.Addr, rt.Label(), os.Getpid())
	}
	if secure != nil {
		fmt.Printf("pida started: https://%s, model %s, PID %d\n", secure.Addr, rt.Label(), os.Getpid())
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
			return fmt.Errorf("https server exited: %w", err)
		}
		return nil
	}
	if err := plain.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server exited: %w", err)
	}
	return nil
}
