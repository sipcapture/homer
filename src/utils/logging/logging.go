// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.
//

package logging

import (
	"io"
	"log"
	"log/slog"
	"log/syslog"
	"os"
	"path/filepath"
	"strings"
	"time"

	rotatelogs "github.com/lestrrat-go/file-rotatelogs"
	"github.com/lmittmann/tint"
	"github.com/sipcapture/homer-core/src/homerconfig"
	"golang.org/x/term"
)

var (
	Logger      *slog.Logger
	AuditLogger *slog.Logger
)

// GetLogger returns the current slog logger (may be nil before init).
func GetLogger() *slog.Logger {
	return Logger
}

// GetAuditLogger returns the current audit logger (may be nil before init).
func GetAuditLogger() *slog.Logger {
	return AuditLogger
}

// InitLogger initializes slog from legacy homerconfig.
func InitLogger() {
	if homerconfig.MainConfig == nil || homerconfig.MainConfig.Setting.LOG_SETTINGS.Level == "" {
		InitLoggerSimple("info", true, false)
		return
	}

	if homerconfig.MainConfig.Setting.LOG_SETTINGS.Level == "" {
		homerconfig.MainConfig.Setting.LOG_SETTINGS.Level = "error"
	}

	level := parseLevel(homerconfig.MainConfig.Setting.LOG_SETTINGS.Level)
	writer := configureWriter(
		homerconfig.MainConfig.Setting.LOG_SETTINGS.Stdout,
		homerconfig.MainConfig.Setting.LOG_SETTINGS.SysLog,
		homerconfig.MainConfig.Setting.LOG_SETTINGS.Json,
	)

	handler := newHandler(writer, level, homerconfig.MainConfig.Setting.LOG_SETTINGS.Json)
	Logger = slog.New(handler)
	Logger.Info("init logging system", "level", level.String())
	slog.SetDefault(Logger)
}

// InitLoggerSimple initializes slog with simple settings.
func InitLoggerSimple(levelStr string, stdout bool, jsonFormat bool) {
	level := parseLevel(levelStr)
	writer := os.Stderr
	if stdout {
		writer = os.Stdout
		log.SetOutput(os.Stdout)
	}

	handler := newHandler(writer, level, jsonFormat)
	Logger = slog.New(handler)
	Logger.Info("init logging system", "level", level.String())
	slog.SetDefault(Logger)
}

// InitAuditLogger initializes audit logger.
func InitAuditLogger() {
	AuditLogger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	AuditLogger.Info("init logging system")
}

func parseLevel(levelStr string) slog.Level {
	switch strings.ToLower(levelStr) {
	case "trace", "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelError
	}
}

func newHandler(writer io.Writer, level slog.Level, jsonFormat bool) slog.Handler {
	opts := &slog.HandlerOptions{Level: level}
	if jsonFormat {
		return slog.NewJSONHandler(writer, opts)
	}
	useColors := wantColors(writer)
	return tint.NewHandler(writer, &tint.Options{
		Level:      level,
		NoColor:    !useColors,
		TimeFormat: time.RFC3339,
	})
}

// wantColors returns true when ANSI colors should be used (TTY or HOMER_LOG_COLORS=1).
func wantColors(writer io.Writer) bool {
	if os.Getenv("HOMER_LOG_COLORS") == "1" {
		return true
	}
	f, ok := writer.(*os.File)
	if !ok {
		return false
	}
	fd := int(f.Fd())
	return term.IsTerminal(fd)
}

func configureWriter(stdout bool, syslogEnabled bool, jsonFormat bool) io.Writer {
	if stdout {
		log.SetOutput(os.Stdout)
		return os.Stdout
	}
	if !syslogEnabled {
		writer := configureLocalFileSystemHook()
		if writer != nil {
			return writer
		}
		return os.Stderr
	}

	writer := configureSyslogHook(jsonFormat)
	if writer != nil {
		return writer
	}
	return os.Stderr
}

func configureLocalFileSystemHook() io.Writer {
	logPath := homerconfig.MainConfig.Setting.LOG_SETTINGS.Path
	logName := homerconfig.MainConfig.Setting.LOG_SETTINGS.Name
	var err error

	if configPath := os.Getenv("HOMER_SERVER_LOGPATH"); configPath != "" {
		logPath = configPath
	}

	if configName := os.Getenv("HOMER_SERVER_LOGNAME"); configName != "" {
		logName = configName
	}

	fileLogExtension := filepath.Ext(logName)
	fileLogBase := strings.TrimSuffix(logName, fileLogExtension)

	pathAllLog := logPath + "/" + fileLogBase + "_%Y%m%d%H%M" + fileLogExtension
	pathLog := logPath + "/" + logName

	rotator, err := rotatelogs.New(
		pathAllLog,
		rotatelogs.WithLinkName(pathLog),
		rotatelogs.WithMaxAge(time.Duration(homerconfig.MainConfig.Setting.LOG_SETTINGS.MaxAgeDays)*time.Hour*24),
		rotatelogs.WithRotationTime(time.Duration(homerconfig.MainConfig.Setting.LOG_SETTINGS.RotationHours)*time.Hour),
	)
	if err != nil {
		log.Printf("Local file system hook initialize fail: %v", err)
		return nil
	}

	log.SetOutput(rotator)
	return rotator
}

func configureSyslogHook(jsonFormat bool) io.Writer {
	sevceritySyslog := getSevirtyByName(homerconfig.MainConfig.Setting.LOG_SETTINGS.SysLogLevel)
	syslogger, err := syslog.New(syslog.Priority(sevceritySyslog), "homer-core")
	if err != nil {
		slog.Error("Unable to connect to syslog", "error", err)
		return nil
	}

	log.SetOutput(syslogger)
	return &syslogWriter{writer: syslogger, jsonFormat: jsonFormat}
}

type syslogWriter struct {
	writer     *syslog.Writer
	jsonFormat bool
}

func (w *syslogWriter) Write(p []byte) (n int, err error) {
	msg := strings.TrimSpace(string(p))
	if msg != "" {
		w.writer.Info(msg)
	}
	return len(p), nil
}

type syslogPriority = syslog.Priority

func getSevirtyByName(name string) syslogPriority {
	switch strings.ToLower(name) {
	case "emerg":
		return syslog.LOG_EMERG
	case "alert":
		return syslog.LOG_ALERT
	case "crit":
		return syslog.LOG_CRIT
	case "err", "error":
		return syslog.LOG_ERR
	case "warning", "warn":
		return syslog.LOG_WARNING
	case "notice":
		return syslog.LOG_NOTICE
	case "info":
		return syslog.LOG_INFO
	case "debug":
		return syslog.LOG_DEBUG
	default:
		return syslog.LOG_INFO
	}
}

func Info(msg string, args ...any) {
	if Logger == nil {
		return
	}
	Logger.Info(msg, args...)
}

func Error(msg string, args ...any) {
	if Logger == nil {
		return
	}
	Logger.Error(msg, args...)
}

func Debug(msg string, args ...any) {
	if Logger == nil {
		return
	}
	Logger.Debug(msg, args...)
}

func Warn(msg string, args ...any) {
	if Logger == nil {
		return
	}
	Logger.Warn(msg, args...)
}

func AuditInfoHmac(auditType, hmac, msg string) {
	if AuditLogger != nil {
		AuditLogger.Info(msg, "auditype", auditType, "signature", hmac)
		return
	}
	if Logger != nil {
		Logger.Info(msg, "auditype", auditType, "signature", hmac)
	}
}

func AuditInfo(auditType, msg string) {
	if AuditLogger != nil {
		AuditLogger.Info(msg, "auditype", auditType)
		return
	}
	if Logger != nil {
		Logger.Info(msg, "auditype", auditType)
	}
}
