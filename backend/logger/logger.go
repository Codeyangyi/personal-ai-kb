package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// LogLevel 日志级别
type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
)

var levelNames = map[LogLevel]string{
	DEBUG: "DEBUG",
	INFO:  "INFO",
	WARN:  "WARN",
	ERROR: "ERROR",
}

// Logger 日志记录器
type Logger struct {
	level        LogLevel
	logDir       string
	logFile      *os.File
	errorFile    *os.File
	currentDate  string
	consoleOut   bool
	mu           sync.Mutex
	infoLogger   *log.Logger
	errorLogger  *log.Logger
}

var defaultLogger *Logger
var once sync.Once

// ParseLevel 解析日志级别字符串
func ParseLevel(levelStr string) LogLevel {
	switch levelStr {
	case "debug", "DEBUG":
		return DEBUG
	case "info", "INFO":
		return INFO
	case "warn", "WARN", "warning", "WARNING":
		return WARN
	case "error", "ERROR":
		return ERROR
	default:
		return INFO // 默认使用 INFO
	}
}

// Init 初始化日志系统
func Init(logDir string, level LogLevel, consoleOut bool) error {
	var err error
	once.Do(func() {
		defaultLogger, err = newLogger(logDir, level, consoleOut)
	})
	return err
}

// newLogger 创建新的日志记录器
func newLogger(logDir string, level LogLevel, consoleOut bool) (*Logger, error) {
	// 确保日志目录存在
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("创建日志目录失败: %v", err)
	}

	l := &Logger{
		level:      level,
		logDir:     logDir,
		consoleOut: consoleOut,
	}

	// 初始化日志文件
	if err := l.rotateLogs(); err != nil {
		return nil, err
	}

	// 启动日志轮转检查协程（每天检查一次）
	go l.startRotationChecker()

	return l, nil
}

// rotateLogs 轮转日志文件（按日期）
func (l *Logger) rotateLogs() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	today := time.Now().Format("2006-01-02")

	// 如果日期没变，不需要轮转
	if l.currentDate == today && l.logFile != nil {
		return nil
	}

	// 关闭旧文件
	if l.logFile != nil {
		l.logFile.Close()
	}
	if l.errorFile != nil {
		l.errorFile.Close()
	}

	// 创建新的日志文件
	logFileName := filepath.Join(l.logDir, fmt.Sprintf("app-%s.log", today))
	errorFileName := filepath.Join(l.logDir, fmt.Sprintf("error-%s.log", today))

	var err error
	l.logFile, err = os.OpenFile(logFileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return fmt.Errorf("打开日志文件失败: %v", err)
	}

	l.errorFile, err = os.OpenFile(errorFileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return fmt.Errorf("打开错误日志文件失败: %v", err)
	}

	// 创建多写入器（同时写入文件和控制台）
	var logWriter io.Writer = l.logFile
	var errorWriter io.Writer = l.errorFile

	if l.consoleOut {
		logWriter = io.MultiWriter(l.logFile, os.Stdout)
		errorWriter = io.MultiWriter(l.errorFile, os.Stderr)
	}

	// 创建日志记录器
	l.infoLogger = log.New(logWriter, "", log.LstdFlags|log.Lmicroseconds)
	l.errorLogger = log.New(errorWriter, "", log.LstdFlags|log.Lmicroseconds)

	l.currentDate = today
	return nil
}

// startRotationChecker 启动日志轮转检查协程
func (l *Logger) startRotationChecker() {
	ticker := time.NewTicker(1 * time.Hour) // 每小时检查一次
	defer ticker.Stop()

	for range ticker.C {
		today := time.Now().Format("2006-01-02")
		if l.currentDate != today {
			l.rotateLogs()
		}
	}
}

// log 记录日志（内部方法）
func (l *Logger) log(level LogLevel, format string, args ...interface{}) {
	if level < l.level {
		return
	}

	// 确保日志文件是最新的
	l.rotateLogs()

	levelName := levelNames[level]
	message := fmt.Sprintf(format, args...)
	timestamp := time.Now().Format("2006-01-02 15:04:05.000")
	logMessage := fmt.Sprintf("[%s] [%s] %s", timestamp, levelName, message)

	l.mu.Lock()
	defer l.mu.Unlock()

	if level >= ERROR {
		// 错误级别写入错误日志文件
		l.errorLogger.Println(logMessage)
	} else {
		// 其他级别写入普通日志文件
		l.infoLogger.Println(logMessage)
	}
}

// Debug 记录调试日志
func (l *Logger) Debug(format string, args ...interface{}) {
	l.log(DEBUG, format, args...)
}

// Info 记录信息日志
func (l *Logger) Info(format string, args ...interface{}) {
	l.log(INFO, format, args...)
}

// Warn 记录警告日志
func (l *Logger) Warn(format string, args ...interface{}) {
	l.log(WARN, format, args...)
}

// Error 记录错误日志
func (l *Logger) Error(format string, args ...interface{}) {
	l.log(ERROR, format, args...)
}

// Fatal 记录致命错误并退出
func (l *Logger) Fatal(format string, args ...interface{}) {
	l.log(ERROR, format, args...)
	os.Exit(1)
}

// Close 关闭日志文件
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	var err error
	if l.logFile != nil {
		if e := l.logFile.Close(); e != nil {
			err = e
		}
	}
	if l.errorFile != nil {
		if e := l.errorFile.Close(); e != nil {
			err = e
		}
	}
	return err
}

// 全局日志函数（使用默认日志记录器）

// Debug 记录调试日志
func Debug(format string, args ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.Debug(format, args...)
	}
}

// Info 记录信息日志
func Info(format string, args ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.Info(format, args...)
	}
}

// Warn 记录警告日志
func Warn(format string, args ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.Warn(format, args...)
	}
}

// Error 记录错误日志
func Error(format string, args ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.Error(format, args...)
	}
}

// Fatal 记录致命错误并退出
func Fatal(format string, args ...interface{}) {
	if defaultLogger != nil {
		defaultLogger.Fatal(format, args...)
	} else {
		log.Fatalf(format, args...)
	}
}

// Printf 兼容标准 log.Printf
func Printf(format string, args ...interface{}) {
	Info(format, args...)
}

// Println 兼容标准 log.Println
func Println(args ...interface{}) {
	Info("%s", fmt.Sprintln(args...))
}

// Fatalf 兼容标准 log.Fatalf
func Fatalf(format string, args ...interface{}) {
	Fatal(format, args...)
}

// Close 关闭日志文件
func Close() error {
	if defaultLogger != nil {
		return defaultLogger.Close()
	}
	return nil
}

