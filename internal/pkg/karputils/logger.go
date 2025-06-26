package karputils

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"golang.org/x/term"
)

var (
	LogLevel  slog.LevelVar
	LogLevels = map[string]slog.Level{
		"debug": slog.LevelDebug,
		"info":  slog.LevelInfo,
		"warn":  slog.LevelWarn,
		"error": slog.LevelError,
	}
)

func SetLogLevel(level string) error {
	if newLevel, ok := LogLevels[level]; ok {
		LogLevel.Set(newLevel)
		return nil
	} else {
		return fmt.Errorf("invalid logging level '%v'", level)
	}
}

func SetupLogger() {

	http.HandleFunc("/log", func(w http.ResponseWriter, r *http.Request) {
		level := r.URL.Query().Get("level")
		if err := SetLogLevel(level); err == nil {
			http.Error(w, "Ok", http.StatusOK)
		} else {
			http.Error(w, err.Error(), http.StatusBadRequest)
		}
	})

	LogLevel.Set(slog.LevelDebug)

	var logger *slog.Logger
	if term.IsTerminal(int(os.Stdout.Fd())) {
		logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: &LogLevel}))
	} else {
		logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: &LogLevel}))
	}

	slog.SetDefault(logger)
}

func MessageFmt(id uint64, topic string, partition int32, offset int64) string {
	if len(topic) > 0 {
		return fmt.Sprintf("%v#%v[%v]@%v", id, topic, partition, offset)
	} else {
		return fmt.Sprintf("%v#PONG", id)
	}
}
