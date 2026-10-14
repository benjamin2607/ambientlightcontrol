// ambientlightd runs the backlight automation and the API for the macOS app.
//
// Configuration (environment):
//
//	GOVEE_API_KEY / GOVEE_API_KEY_FILE   Govee developer API key (required)
//	API_TOKEN / API_TOKEN_FILE           token for the app; generated into $CONFIG_DIR/api-token if unset
//	GOVEE_DEVICE                         device id or name, if the account has several devices
//	TV_HOST                              IP address of the LG TV (can also be set from the app)
//	CONFIG_DIR                           state directory (default /config)
//	LISTEN_ADDR                          default :8787
//	LOG_LEVEL                            debug, info (default), warn
//	TZ                                   time zone for the schedule, e.g. Europe/Berlin
package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	_ "time/tzdata" // the container image has no zoneinfo

	"github.com/benjamin2607/ambientlightcontrol/server/internal/api"
	"github.com/benjamin2607/ambientlightcontrol/server/internal/controller"
	"github.com/benjamin2607/ambientlightcontrol/server/internal/govee"
	"github.com/benjamin2607/ambientlightcontrol/server/internal/store"
	"github.com/benjamin2607/ambientlightcontrol/server/internal/webos"
)

// Set at build time: -ldflags "-X main.version=1.0.0".
var version = "dev"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "healthcheck":
			os.Exit(healthcheck())
		case "version":
			fmt.Println(version)
			return
		}
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel()}))

	configDir := envOr("CONFIG_DIR", "/config")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		return err
	}
	if err := checkWritable(configDir); err != nil {
		return err
	}
	apiKey, err := secret("GOVEE_API_KEY")
	if err != nil {
		return err
	}
	if apiKey == "" {
		return errors.New("GOVEE_API_KEY is not set")
	}
	token, err := apiToken(configDir, log)
	if err != nil {
		return err
	}

	st, err := store.Open(configDir)
	if err != nil {
		return err
	}
	if host := os.Getenv("TV_HOST"); host != "" && st.Settings().TVHost == "" {
		if _, err := st.UpdateSettings(func(s *store.Settings) { s.TVHost = host }); err != nil {
			return err
		}
	}

	ctrl := controller.New(log, govee.NewClient(apiKey), st, webos.Watch, os.Getenv("GOVEE_DEVICE"))
	srv := api.New(ctrl, token, version, log).HTTPServer(envOr("LISTEN_ADDR", ":8787"))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go ctrl.Run(ctx)

	errc := make(chan error, 1)
	go func() {
		log.Info("ambientlightd started", "version", version, "addr", srv.Addr, "timezone", time.Local.String())
		errc <- srv.ListenAndServe()
	}()
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

// apiToken reads API_TOKEN(_FILE) or a token generated on first start. The token itself is never logged.
func apiToken(configDir string, log *slog.Logger) (string, error) {
	token, err := secret("API_TOKEN")
	if err != nil || token != "" {
		if err == nil && len(token) < 24 {
			return "", errors.New("API_TOKEN must be at least 24 characters")
		}
		return token, err
	}
	path := filepath.Join(configDir, "api-token")
	data, err := os.ReadFile(path)
	if err == nil && len(strings.TrimSpace(string(data))) >= 24 {
		return strings.TrimSpace(string(data)), nil
	}
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token = base64.RawURLEncoding.EncodeToString(raw)
	if err := os.WriteFile(path, []byte(token+"\n"), 0o600); err != nil {
		return "", err
	}
	log.Info("generated a new API token; copy it from the config directory into the app", "file", path)
	return token, nil
}

// checkWritable fails early with a hint, because Docker creates missing host folders as root.
func checkWritable(dir string) error {
	f, err := os.CreateTemp(dir, ".write-test-*")
	if err != nil {
		return fmt.Errorf("cannot write to %s (running as uid %d). On the host, run: chown -R %d:%d <config folder>",
			dir, os.Getuid(), os.Getuid(), os.Getgid())
	}
	f.Close()
	return os.Remove(f.Name())
}

// secret reads NAME, or the file named by NAME_FILE (Docker secrets).
func secret(name string) (string, error) {
	if file := os.Getenv(name + "_FILE"); file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			return "", fmt.Errorf("%s_FILE: %w", name, err)
		}
		return strings.TrimSpace(string(data)), nil
	}
	return strings.TrimSpace(os.Getenv(name)), nil
}

func envOr(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

func logLevel() slog.Level {
	switch strings.ToLower(os.Getenv("LOG_LEVEL")) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	default:
		return slog.LevelInfo
	}
}

// healthcheck is used by the Docker HEALTHCHECK (the image has no shell or curl).
func healthcheck() int {
	addr := envOr("LISTEN_ADDR", ":8787")
	if strings.HasPrefix(addr, ":") {
		addr = "127.0.0.1" + addr
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://" + addr + "/healthz")
	if err != nil {
		return 1
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}
