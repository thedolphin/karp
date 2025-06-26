package karputils

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
)

type SlogGrpcLogger struct {
	log *slog.Logger
}

func (s *SlogGrpcLogger) Info(args ...any) {
	s.log.Info("gRPC: " + strings.TrimSuffix(fmt.Sprint(args...), "\n"))
}

func (s *SlogGrpcLogger) Infoln(args ...any) {
	s.log.Info("gRPC: " + strings.TrimSuffix(fmt.Sprintln(args...), "\n"))
}

func (s *SlogGrpcLogger) Warning(args ...any) {
	s.log.Warn("gRPC: " + strings.TrimSuffix(fmt.Sprint(args...), "\n"))
}

func (s *SlogGrpcLogger) Warningln(args ...any) {
	s.log.Warn("gRPC: " + strings.TrimSuffix(fmt.Sprintln(args...), "\n"))
}

func (s *SlogGrpcLogger) Error(args ...any) {
	s.log.Error("gRPC: " + strings.TrimSuffix(fmt.Sprint(args...), "\n"))
}

func (s *SlogGrpcLogger) Errorln(args ...any) {
	s.log.Error("gRPC: " + strings.TrimSuffix(fmt.Sprintln(args...), "\n"))
}

func (s *SlogGrpcLogger) Infof(format string, args ...any) {
	s.log.Info("gRPC: " + strings.TrimSuffix(fmt.Sprintf(format, args...), "\n"))
}

func (s *SlogGrpcLogger) Warningf(format string, args ...any) {
	s.log.Warn("gRPC: " + strings.TrimSuffix(fmt.Sprintf(format, args...), "\n"))
}

func (s *SlogGrpcLogger) Errorf(format string, args ...any) {
	s.log.Error("gRPC: " + strings.TrimSuffix(fmt.Sprintf(format, args...), "\n"))
}

func (s *SlogGrpcLogger) Fatal(args ...any) {
	s.log.Error("gRPC: " + strings.TrimSuffix(fmt.Sprint(args...), "\n"))
	os.Exit(1)
}

func (s *SlogGrpcLogger) Fatalln(args ...any) {
	s.log.Error("gRPC: " + strings.TrimSuffix(fmt.Sprintln(args...), "\n"))
	os.Exit(1)
}

func (s *SlogGrpcLogger) Fatalf(format string, args ...any) {
	s.log.Error("gRPC: " + strings.TrimSuffix(fmt.Sprintf(format, args...), "\n"))
	os.Exit(1)
}

// V reports whether verbosity level l is at least the requested verbose level.
func (s *SlogGrpcLogger) V(l int) bool {
	return true
}

func NewSlogGrpcLogger() *SlogGrpcLogger {
	return &SlogGrpcLogger{log: slog.Default()}
}
