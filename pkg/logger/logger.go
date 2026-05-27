package logger

import (
	"log/slog"
	"os"
	"time"
)

type Logger struct {
	logger *slog.Logger
}

func New() *Logger {
	level := slog.LevelInfo
	opts := &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				return slog.Attr{
					Key:   "timestamp",
					Value: slog.StringValue(a.Value.Time().Format(time.RFC3339)),
				}
			}
			return a
		},
	}

	handler := slog.NewJSONHandler(os.Stdout, opts)
	return &Logger{
		logger: slog.New(handler),
	}
}

func (l *Logger) With(args ...any) *Logger {
	return &Logger{
		l.logger.With(args...),
	}
}

func (l *Logger) Info(message string, args ...any) {
	l.logger.Info(message, args...)
}

func (l *Logger) Debug(message string, args ...any) {
	l.logger.Debug(message, args...)
}

func (l *Logger) Warn(message string, args ...any) {
	l.logger.Warn(message, args...)
}

func (l *Logger) Error(message string, args ...any) {
	l.logger.Error(message, args...)
}

func (l *Logger) Fatal(message string, args ...any) {
	l.logger.Error(message, args...)
	os.Exit(1)
}
