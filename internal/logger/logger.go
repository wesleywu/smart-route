package logger

import (
	"log/slog"
	"os"
	"strings"
)

type Logger struct {
	*slog.Logger
}

func New(logLevel string) *Logger {
	opts := &slog.HandlerOptions{
		Level: parseLogLevel(logLevel),
		AddSource: logLevel == "debug",
	}

	handler := slog.NewJSONHandler(os.Stdout, opts)

	return &Logger{
		Logger: slog.New(handler),
	}
}

func parseLogLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func (l *Logger) WithComponent(component string) *Logger {
	return &Logger{
		Logger: l.Logger.With("component", component),
	}
}

func (l *Logger) WithFields(fields ...interface{}) *Logger {
	return &Logger{
		Logger: l.Logger.With(fields...),
	}
}

func (l *Logger) RouteOperation(action, network, gateway string, duration int64, success bool) {
	l.Info("Route operation completed",
		slog.String("action", action),
		slog.String("network", network),
		slog.String("gateway", gateway),
		slog.Int64("duration_ms", duration),
		slog.Bool("success", success))
}

func (l *Logger) NetworkChange(eventType, iface, oldGateway, newGateway string) {
	l.Info("Network change detected",
		slog.String("event", eventType),
		slog.String("interface", iface),
		slog.String("old_gateway", oldGateway),
		slog.String("new_gateway", newGateway))
}

func (l *Logger) ServiceStart(version, pid string) {
	l.Info("Service starting",
		slog.String("version", version),
		slog.String("pid", pid))
}

func (l *Logger) ServiceStop() {
	l.Info("Service stopping")
}

func (l *Logger) BatchOperation(action string, total, success, failed int, duration int64) {
	l.Info("Batch operation completed",
		slog.String("action", action),
		slog.Int("total", total),
		slog.Int("success", success),
		slog.Int("failed", failed),
		slog.Int64("duration_ms", duration))
}

func (l *Logger) ConfigLoaded(file string, routes, dns int) {
	l.Info("Configuration loaded",
		slog.String("config_file", file),
		slog.Int("chn_routes", routes),
		slog.Int("chn_dns", dns))
}

func (l *Logger) MonitorStart(interval string) {
	l.Info("Network monitor started",
		slog.String("poll_interval", interval))
}

func (l *Logger) MonitorStop() {
	l.Info("Network monitor stopped")
}

func (l *Logger) Performance(operation string, metrics map[string]interface{}) {
	args := []interface{}{
		"operation", operation,
	}
	
	for k, v := range metrics {
		args = append(args, k, v)
	}
	
	l.Debug("performance metrics", args...)
}