package main

import (
	"log/slog"
	"net/http"
	"os"
)

func getEnvOrDefault(key string, fallback string) string {
	v, ok := os.LookupEnv(key)
	if ok {
		return v
	}
	return fallback
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", notFoundHandler)
	mux.HandleFunc("POST /time-recorder/toggle", TimeRecorderToggle)
	mux.HandleFunc("POST /time-recorder/status", TimeRecorderStatus)

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	logged := loggingMiddleware(logger)(mux)

	addr := ":" + getEnvOrDefault("PORT", "8080")
	slog.Info("Starting server...", "addr", addr)
	if err := http.ListenAndServe(addr, logged); err != nil {
		slog.Error("Server failed", "err", err)
	}
}
