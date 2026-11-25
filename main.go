package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/liyu1981/pi-durable-assistant/internal/config"
	"github.com/liyu1981/pi-durable-assistant/internal/httpapi"
	"github.com/liyu1981/pi-durable-assistant/internal/runtime"
)

//go:embed web/dist
var webDist embed.FS

func main() {
	cfg := config.Load()
	if cfg.Production && cfg.Password == "" {
		fmt.Fprintln(os.Stderr, "生产环境必须设置 DEMO_PASSWORD")
		os.Exit(1)
	}
	rt, err := runtime.Open(context.Background(), cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "启动失败：", cfg.Redact(err.Error()))
		os.Exit(1)
	}
	assets, err := fs.Sub(webDist, "web/dist")
	if err != nil {
		fmt.Fprintln(os.Stderr, "静态资源加载失败：", err)
		os.Exit(1)
	}
	server, err := httpapi.NewServer(rt, assets)
	if err != nil {
		fmt.Fprintln(os.Stderr, "HTTP 初始化失败：", err)
		os.Exit(1)
	}
	host := "127.0.0.1"
	if cfg.Password != "" {
		host = "0.0.0.0"
	}
	httpServer := &http.Server{
		Addr:              fmt.Sprintf("%s:%d", host, cfg.Port),
		Handler:           server,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-stop
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		httpServer.Shutdown(ctx)
	}()
	fmt.Printf("pi-durable-assistant 已启动：%s，模型 %s，PID %d\n", httpServer.Addr, rt.ModelLabel, os.Getpid())
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Fprintln(os.Stderr, "服务退出：", err)
		os.Exit(1)
	}
}
