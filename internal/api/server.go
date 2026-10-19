package api

import (
	"context"
	"embed"
	"errors"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/Jack-Overflow/Evigo/internal/history"
	"github.com/Jack-Overflow/Evigo/internal/snapshot"
)

//go:embed all:static
var assets embed.FS

type ReadHistory func(ctx context.Context, limit int, beforeID int64) (history.Page, error)

type Settings struct {
	Listen        string
	AllowedHosts  []string
	Version       string
	UPS           string
	HealthyWithin time.Duration
}

type Server struct {
	settings       Settings
	latestSnapshot func() snapshot.Snapshot
	readHistory    ReadHistory
	now            func() time.Time
}

func New(settings Settings, latestSnapshot func() snapshot.Snapshot, readHistory ReadHistory) *Server {
	return &Server{settings: settings, latestSnapshot: latestSnapshot, readHistory: readHistory, now: time.Now}
}

func (server *Server) Handler() http.Handler {
	routes := http.NewServeMux()
	routes.HandleFunc("GET /health", server.health)
	routes.HandleFunc("GET /api/status", server.status)
	routes.HandleFunc("GET /api/history", server.history)
	routes.HandleFunc("GET /api/", func(writer http.ResponseWriter, _ *http.Request) {
		writeError(writer, http.StatusNotFound, "not_found")
	})
	routes.Handle("/", staticFiles())

	return server.protect(routes)
}

func (server *Server) Serve(ctx context.Context, listener net.Listener) error {
	httpServer := &http.Server{
		Handler:           server.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 * 1024,
	}
	shutdownDone := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		defer close(shutdownDone)
		shutdownContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if httpServer.Shutdown(shutdownContext) != nil {
			httpServer.Close()
		}
	})

	err := httpServer.Serve(listener)
	if !errors.Is(err, http.ErrServerClosed) {
		stop()

		return err
	}

	<-shutdownDone

	return nil
}

func staticFiles() http.Handler {
	files, _ := fs.Sub(assets, "static")
	fileServer := http.FileServer(http.FS(files))
	_, missing := fs.Stat(files, "index.html")

	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method != http.MethodGet && request.Method != http.MethodHead:
			writer.WriteHeader(http.StatusMethodNotAllowed)
		case strings.HasPrefix(request.URL.Path, "/."):
			http.NotFound(writer, request)
		case missing != nil:
			http.Error(writer, "Dashboard files are missing. Build Evigo with make build.", http.StatusServiceUnavailable)
		default:
			fileServer.ServeHTTP(writer, request)
		}
	})
}
