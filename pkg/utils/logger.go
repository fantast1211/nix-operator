package utils

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"
)

// LogLevel 定义日志级别
type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
	FATAL
)

// String 返回日志级别的字符串表示
func (l LogLevel) String() string {
	switch l {
	case DEBUG:
		return "DEBUG"
	case INFO:
		return "INFO"
	case WARN:
		return "WARN"
	case ERROR:
		return "ERROR"
	case FATAL:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

// Logger 日志记录器结构体
type Logger struct {
	serviceName string
	infoLogger  *log.Logger
	errorLogger *log.Logger
	infoFile    *os.File
	errorFile   *os.File
}

// NewLogger 创建新的日志记录器
func NewLogger(serviceName string) (*Logger, error) {
	// 确保日志目录存在
	logDir := "/opt/log"
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %v", err)
	}

	// 打开普通日志文件
	infoLogPath := filepath.Join(logDir, "xtopus.log")
	infoFile, err := os.OpenFile(infoLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open info log file: %v", err)
	}

	// 打开错误日志文件
	errorLogPath := filepath.Join(logDir, "xtopus_error.log")
	errorFile, err := os.OpenFile(errorLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		infoFile.Close()
		return nil, fmt.Errorf("failed to open error log file: %v", err)
	}

	// 创建多重写入器，同时写入文件和控制台
	infoMultiWriter := io.MultiWriter(infoFile, os.Stdout)
	errorMultiWriter := io.MultiWriter(errorFile, os.Stderr)

	// 创建日志记录器
	infoLogger := log.New(infoMultiWriter, "", 0)
	errorLogger := log.New(errorMultiWriter, "", 0)

	return &Logger{
		serviceName: serviceName,
		infoLogger:  infoLogger,
		errorLogger: errorLogger,
		infoFile:    infoFile,
		errorFile:   errorFile,
	}, nil
}

// formatMessage 格式化日志消息
func (l *Logger) formatMessage(level LogLevel, module, message string) string {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	if module != "" {
		return fmt.Sprintf("[%s] [%s] [%s] %s", timestamp, level.String(), module, message)
	}
	return fmt.Sprintf("[%s] [%s] %s", timestamp, level.String(), message)
}

// Debug 记录调试日志
func (l *Logger) Debug(module, message string) {
	formatted := l.formatMessage(DEBUG, module, message)
	l.infoLogger.Println(formatted)
}

// Debugf 记录格式化调试日志
func (l *Logger) Debugf(module, format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	l.Debug(module, message)
}

// Info 记录信息日志
func (l *Logger) Info(module, message string) {
	formatted := l.formatMessage(INFO, module, message)
	l.infoLogger.Println(formatted)
}

// Infof 记录格式化信息日志
func (l *Logger) Infof(module, format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	l.Info(module, message)
}

// Warn 记录警告日志
func (l *Logger) Warn(module, message string) {
	formatted := l.formatMessage(WARN, module, message)
	l.infoLogger.Println(formatted)
}

// Warnf 记录格式化警告日志
func (l *Logger) Warnf(module, format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	l.Warn(module, message)
}

// Error 记录错误日志
func (l *Logger) Error(module, message string) {
	formatted := l.formatMessage(ERROR, module, message)
	l.errorLogger.Println(formatted)
}

// Errorf 记录格式化错误日志
func (l *Logger) Errorf(module, format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	l.Error(module, message)
}

// Fatal 记录致命错误日志并退出程序
func (l *Logger) Fatal(module, message string) {
	formatted := l.formatMessage(FATAL, module, message)
	l.errorLogger.Println(formatted)
	os.Exit(1)
}

// Fatalf 记录格式化致命错误日志并退出程序
func (l *Logger) Fatalf(module, format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	l.Fatal(module, message)
}

// Close 关闭日志文件
func (l *Logger) Close() error {
	var err error
	if l.infoFile != nil {
		if closeErr := l.infoFile.Close(); closeErr != nil {
			err = closeErr
		}
	}
	if l.errorFile != nil {
		if closeErr := l.errorFile.Close(); closeErr != nil {
			err = closeErr
		}
	}
	return err
}

// 全局日志实例
var GlobalLogger *Logger

// InitLogger 初始化全局日志实例
func InitLogger(serviceName string) error {
	logger, err := NewLogger(serviceName)
	if err != nil {
		return err
	}
	GlobalLogger = logger
	return nil
}

// 便捷的全局日志函数
func Debug(module, message string) {
	if GlobalLogger != nil {
		GlobalLogger.Debug(module, message)
	}
}

func Debugf(module, format string, args ...interface{}) {
	if GlobalLogger != nil {
		GlobalLogger.Debugf(module, format, args...)
	}
}

func Info(module, message string) {
	if GlobalLogger != nil {
		GlobalLogger.Info(module, message)
	}
}

func Infof(module, format string, args ...interface{}) {
	if GlobalLogger != nil {
		GlobalLogger.Infof(module, format, args...)
	}
}

func Warn(module, message string) {
	if GlobalLogger != nil {
		GlobalLogger.Warn(module, message)
	}
}

func Warnf(module, format string, args ...interface{}) {
	if GlobalLogger != nil {
		GlobalLogger.Warnf(module, format, args...)
	}
}

func Error(module, message string) {
	if GlobalLogger != nil {
		GlobalLogger.Error(module, message)
	}
}

func Errorf(module, format string, args ...interface{}) {
	if GlobalLogger != nil {
		GlobalLogger.Errorf(module, format, args...)
	}
}

func Fatal(module, message string) {
	if GlobalLogger != nil {
		GlobalLogger.Fatal(module, message)
	}
}

func Fatalf(module, format string, args ...interface{}) {
	if GlobalLogger != nil {
		GlobalLogger.Fatalf(module, format, args...)
	}
}

// CloseLogger 关闭全局日志实例
func CloseLogger() error {
	if GlobalLogger != nil {
		return GlobalLogger.Close()
	}
	return nil
}