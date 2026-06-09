// Package logger 实现结构化日志输出
package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"
)

const (
	// DefaultLogDir 默认日志目录
	DefaultLogDir = "/var/log/block-area-bot"
	// DefaultLogFile 默认日志文件
	DefaultLogFile = "/var/log/block-area-bot/block-area-bot.log"
)

// Level 日志级别
type Level int

const (
	DEBUG Level = iota
	INFO
	WARN
	ERROR
)

func (l Level) String() string {
	switch l {
	case DEBUG:
		return "DEBUG"
	case INFO:
		return "INFO"
	case WARN:
		return "WARN"
	case ERROR:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// Logger 日志记录器
type Logger struct {
	level  Level
	logger *log.Logger
	file   *os.File
}

var defaultLogger *Logger

// Init 初始化日志系统
// daemon 模式写入文件，CLI 模式输出到 stderr
func Init(daemonMode bool) error {
	var writer io.Writer

	if daemonMode {
		// 确保日志目录存在
		if err := os.MkdirAll(DefaultLogDir, 0755); err != nil {
			return fmt.Errorf("创建日志目录失败: %w", err)
		}

		// 打开日志文件
		logPath := DefaultLogFile
		f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return fmt.Errorf("打开日志文件失败: %w", err)
		}

		// 同时输出到文件和 stderr
		writer = io.MultiWriter(f, os.Stderr)
		defaultLogger = &Logger{
			level:  INFO,
			logger: log.New(writer, "", 0),
			file:   f,
		}
	} else {
		writer = os.Stderr
		defaultLogger = &Logger{
			level:  INFO,
			logger: log.New(writer, "", 0),
		}
	}

	// 同时设置标准 log 包的输出
	log.SetOutput(writer)
	log.SetFlags(0)

	return nil
}

// Close 关闭日志文件
func Close() {
	if defaultLogger != nil && defaultLogger.file != nil {
		defaultLogger.file.Close()
	}
}

// SetLevel 设置日志级别
func SetLevel(level Level) {
	if defaultLogger != nil {
		defaultLogger.level = level
	}
}

// logf 格式化输出日志
func logf(level Level, format string, args ...interface{}) {
	if defaultLogger == nil {
		// 未初始化时使用标准 log
		log.Printf("[%s] %s", level, fmt.Sprintf(format, args...))
		return
	}

	if level < defaultLogger.level {
		return
	}

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	msg := fmt.Sprintf(format, args...)
	defaultLogger.logger.Printf("%s [%s] %s", timestamp, level, msg)
}

// Debug 输出调试日志
func Debug(format string, args ...interface{}) {
	logf(DEBUG, format, args...)
}

// Info 输出信息日志
func Info(format string, args ...interface{}) {
	logf(INFO, format, args...)
}

// Warn 输出警告日志
func Warn(format string, args ...interface{}) {
	logf(WARN, format, args...)
}

// Error 输出错误日志
func Error(format string, args ...interface{}) {
	logf(ERROR, format, args...)
}

// GetLogPath 获取日志文件路径
func GetLogPath() string {
	return filepath.Join(DefaultLogDir, "block-area-bot.log")
}
