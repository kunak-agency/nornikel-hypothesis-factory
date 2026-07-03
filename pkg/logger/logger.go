// Package logger — структурированный JSON-логгер с прокидыванием request_id
// через context.Context (кладётся в Locals Fiber-мидлварой, читается здесь).
package logger

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
)

type contextKey string

const RequestIDKey contextKey = "request_id"

func logEntry(severity string, ctx context.Context, format string, v ...interface{}) {
	entry := map[string]interface{}{
		"severity": severity,
		"message":  fmt.Sprintf(format, v...),
	}
	if ctx != nil {
		if reqID, ok := ctx.Value(RequestIDKey).(string); ok && reqID != "" {
			entry["request_id"] = reqID
		}
	}
	jsonBytes, err := json.Marshal(entry)
	if err != nil {
		log.Printf("failed to marshal entry: %v, err: %v", entry, err)
		return
	}
	log.Println(string(jsonBytes))
}

func LogInfo(format string, v ...interface{})     { logEntry("INFO", nil, format, v...) }
func LogWarning(format string, v ...interface{})  { logEntry("WARNING", nil, format, v...) }
func LogError(format string, v ...interface{})    { logEntry("ERROR", nil, format, v...) }
func LogCritical(format string, v ...interface{}) { logEntry("CRITICAL", nil, format, v...) }

func LogInfoCtx(ctx context.Context, format string, v ...interface{}) {
	logEntry("INFO", ctx, format, v...)
}
func LogWarningCtx(ctx context.Context, format string, v ...interface{}) {
	logEntry("WARNING", ctx, format, v...)
}
func LogErrorCtx(ctx context.Context, format string, v ...interface{}) {
	logEntry("ERROR", ctx, format, v...)
}
