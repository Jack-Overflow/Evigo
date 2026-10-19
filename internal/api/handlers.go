package api

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/Jack-Overflow/Evigo/internal/recovery"
	"github.com/Jack-Overflow/Evigo/internal/snapshot"
)

const (
	defaultHistoryLimit = 50
	maxHistoryLimit     = 200
	historyTimeout      = 3 * time.Second
)

type monitorState string

const (
	monitorStarting monitorState = "starting"
	monitorStalled  monitorState = "stalled"
	monitorRunning  monitorState = "running"
)

func (server *Server) monitorState(latest snapshot.Snapshot) monitorState {
	switch {
	case latest.CheckedAt.IsZero():
		return monitorStarting
	case server.now().Sub(latest.CheckedAt) > server.settings.HealthyWithin:
		return monitorStalled
	}

	return monitorRunning
}

func (server *Server) health(writer http.ResponseWriter, _ *http.Request) {
	latest := server.latestSnapshot()
	if state := server.monitorState(latest); state != monitorRunning {
		writeJSON(writer, http.StatusServiceUnavailable, map[string]monitorState{"monitor": state})

		return
	}

	upsAvailable := latest.PowerBlock(server.now()) != recovery.NoReading
	writeJSON(writer, http.StatusOK, map[string]any{"monitor": monitorRunning, "ups_available": upsAvailable})
}

func (server *Server) status(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, server.statusResponse(server.latestSnapshot()))
}

func (server *Server) history(writer http.ResponseWriter, request *http.Request) {
	limit, limitErr := queryInt(request, "limit", defaultHistoryLimit)
	beforeID, beforeErr := queryInt(request, "before", 0)
	if limitErr != nil || beforeErr != nil || limit < 1 || limit > maxHistoryLimit || beforeID < 0 {
		writeError(writer, http.StatusBadRequest, "invalid_request")

		return
	}

	ctx, cancel := context.WithTimeout(request.Context(), historyTimeout)
	defer cancel()
	page, err := server.readHistory(ctx, limit, int64(beforeID))
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "unavailable")

		return
	}

	writeJSON(writer, http.StatusOK, historyResponse(page))
}

func queryInt(request *http.Request, name string, fallback int) (int, error) {
	value := request.URL.Query().Get(name)
	if value == "" {
		return fallback, nil
	}

	return strconv.Atoi(value)
}
