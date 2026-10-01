package logging_test

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	dblogger "github.com/jeremyhahn/dragonboat/v4/logger"

	"github.com/lyonbrown4d/orch/internal/logging"
)

func TestDragonboatSlogLoggerWritesStructuredRecord(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	adapter := logging.NewDragonboatLogger(logger, "raft")

	adapter.Infof("node %d ready", 1)

	got := buf.String()
	for _, want := range []string{
		`"msg":"node 1 ready"`,
		`"component":"dragonboat"`,
		`"dragonboat_package":"raft"`,
		`"dragonboat_level":"info"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected log output to contain %s, got %s", want, got)
		}
	}
}

func TestDragonboatSlogLoggerFiltersByDragonboatLevel(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	adapter := logging.NewDragonboatLogger(logger, "raft")
	adapter.SetLevel(dblogger.ERROR)

	adapter.Warningf("skip warning")
	adapter.Errorf("keep error")

	got := buf.String()
	if strings.Contains(got, "skip warning") {
		t.Fatalf("expected warning to be filtered, got %s", got)
	}
	if !strings.Contains(got, `"msg":"keep error"`) {
		t.Fatalf("expected error log to be written, got %s", got)
	}
}
