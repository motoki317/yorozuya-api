package main

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

type simpleMsg struct {
	Message string `json:"message"`
}

func encodeJSON(obj any) ([]byte, error) {
	var buf bytes.Buffer
	err := json.NewEncoder(&buf).Encode(obj)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func respond(w http.ResponseWriter, code int, obj any) {
	resp, err := encodeJSON(obj)
	if err != nil {
		http.Error(w, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		slog.Error("encode error", "err", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, err = w.Write(resp)
	if err != nil {
		slog.Error("write error", "err", err)
	}
}

type wrappedResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *wrappedResponseWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// loggingMiddleware logs each HTTP request via slog.
func loggingMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := &wrappedResponseWriter{ResponseWriter: w, status: http.StatusOK}

			// call the next handler
			next.ServeHTTP(ww, r)

			// log structured record
			logger.LogAttrs(r.Context(), slog.LevelInfo,
				"access",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", ww.status),
				slog.Duration("duration", time.Since(start)),
			)
		})
	}
}

func notFoundHandler(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "Endpoint not found", http.StatusNotFound)
}
