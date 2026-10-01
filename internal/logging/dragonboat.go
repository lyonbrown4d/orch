package logging

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"

	dblogger "github.com/jeremyhahn/dragonboat/v4/logger"
)

var installDragonboatLoggerOnce sync.Once

// InstallDragonboatLogger routes Dragonboat package logs through the process slog logger.
func InstallDragonboatLogger(base *slog.Logger) {
	if base == nil {
		base = slog.Default()
	}
	installDragonboatLoggerOnce.Do(func() {
		defer func() {
			if r := recover(); r != nil {
				base.Warn("dragonboat logger factory already configured", "component", "logging", "recover", r)
			}
		}()
		dblogger.SetLoggerFactory(func(pkgName string) dblogger.ILogger {
			return newDragonboatSlogLogger(base, pkgName)
		})
	})
}

// NewDragonboatLogger creates a slog-backed Dragonboat logger for the package name.
func NewDragonboatLogger(base *slog.Logger, pkgName string) dblogger.ILogger {
	return newDragonboatSlogLogger(base, pkgName)
}

type dragonboatSlogLogger struct {
	logger *slog.Logger
	level  atomic.Int64
}

func newDragonboatSlogLogger(base *slog.Logger, pkgName string) *dragonboatSlogLogger {
	if base == nil {
		base = slog.Default()
	}
	l := &dragonboatSlogLogger{
		logger: base.With(
			"component", "dragonboat",
			"dragonboat_package", pkgName,
		),
	}
	l.SetLevel(dblogger.INFO)
	return l
}

func (l *dragonboatSlogLogger) SetLevel(level dblogger.LogLevel) {
	l.level.Store(int64(level))
}

func (l *dragonboatSlogLogger) Debugf(format string, args ...any) {
	l.logf(dblogger.DEBUG, format, args...)
}

func (l *dragonboatSlogLogger) Infof(format string, args ...any) {
	l.logf(dblogger.INFO, format, args...)
}

func (l *dragonboatSlogLogger) Warningf(format string, args ...any) {
	l.logf(dblogger.WARNING, format, args...)
}

func (l *dragonboatSlogLogger) Errorf(format string, args ...any) {
	l.logf(dblogger.ERROR, format, args...)
}

func (l *dragonboatSlogLogger) Panicf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	l.log(dblogger.CRITICAL, msg)
	panic(msg)
}

func (l *dragonboatSlogLogger) logf(level dblogger.LogLevel, format string, args ...any) {
	if !l.enabled(level) {
		return
	}
	l.log(level, fmt.Sprintf(format, args...))
}

func (l *dragonboatSlogLogger) log(level dblogger.LogLevel, msg string) {
	l.logger.Log(
		context.Background(),
		slogLevelForDragonboat(level),
		msg,
		"dragonboat_level", dragonboatLevelName(level),
	)
}

func (l *dragonboatSlogLogger) enabled(level dblogger.LogLevel) bool {
	return int64(level) <= l.level.Load()
}

func slogLevelForDragonboat(level dblogger.LogLevel) slog.Level {
	switch level {
	case dblogger.DEBUG:
		return slog.LevelDebug
	case dblogger.INFO:
		return slog.LevelInfo
	case dblogger.WARNING:
		return slog.LevelWarn
	case dblogger.ERROR, dblogger.CRITICAL:
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func dragonboatLevelName(level dblogger.LogLevel) string {
	switch level {
	case dblogger.CRITICAL:
		return "critical"
	case dblogger.ERROR:
		return "error"
	case dblogger.WARNING:
		return "warning"
	case dblogger.INFO:
		return "info"
	case dblogger.DEBUG:
		return "debug"
	default:
		return "info"
	}
}
