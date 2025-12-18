// Package logger 提供基于 zap 的日志系统
// 支持控制台输出和按日期、模块分文件的日志
package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// Logger 日志器封装
type Logger struct {
	zap    *zap.SugaredLogger
	prefix string
}

var (
	defaultLogger *Logger
	moduleLoggers sync.Map // 模块日志器缓存
	once          sync.Once
	logDir        = "logs" // 日志根目录
)

// Get 获取默认日志器
func Get() *Logger {
	once.Do(func() {
		defaultLogger = newLogger("", "app")
	})
	return defaultLogger
}

// newLogger 创建日志器
// prefix: 日志前缀显示
// filename: 日志文件名（不含扩展名）
func newLogger(prefix string, filename string) *Logger {
	// 确保日志目录存在
	dateDir := filepath.Join(logDir, time.Now().Format("2006-01-02"))
	if err := os.MkdirAll(dateDir, 0755); err != nil {
		fmt.Printf("创建日志目录失败: %v\n", err)
	}

	// 编码器配置
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "",
		MessageKey:     "msg",
		StacktraceKey:  "",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalLevelEncoder,
		EncodeTime:     zapcore.TimeEncoderOfLayout("2006/01/02 15:04:05"),
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	// 控制台输出
	consoleEncoder := zapcore.NewConsoleEncoder(encoderConfig)
	consoleWriter := zapcore.AddSync(os.Stdout)

	// 模块专用文件
	moduleFile := filepath.Join(dateDir, filename+".log")
	moduleWriter := zapcore.AddSync(&lumberjack.Logger{
		Filename:   moduleFile,
		MaxSize:    100, // MB
		MaxBackups: 30,
		MaxAge:     30, // 天
		Compress:   true,
	})

	// 汇总文件 (所有日志)
	allFile := filepath.Join(dateDir, "all.log")
	allWriter := zapcore.AddSync(&lumberjack.Logger{
		Filename:   allFile,
		MaxSize:    100,
		MaxBackups: 30,
		MaxAge:     30,
		Compress:   true,
	})

	fileEncoder := zapcore.NewJSONEncoder(encoderConfig)

	// 组合输出: 控制台 + 模块文件 + 汇总文件
	core := zapcore.NewTee(
		zapcore.NewCore(consoleEncoder, consoleWriter, zapcore.DebugLevel),
		zapcore.NewCore(fileEncoder, moduleWriter, zapcore.DebugLevel),
		zapcore.NewCore(fileEncoder, allWriter, zapcore.DebugLevel),
	)

	zapLogger := zap.New(core)
	return &Logger{
		zap:    zapLogger.Sugar(),
		prefix: prefix,
	}
}

// WithPrefix 返回带前缀的子日志器（同时创建独立日志文件）
func (l *Logger) WithPrefix(prefix string) *Logger {
	// 检查缓存
	if cached, ok := moduleLoggers.Load(prefix); ok {
		return cached.(*Logger)
	}

	// 创建新的模块日志器
	filename := strings.ToLower(prefix)
	newLogger := newLogger(prefix, filename)

	// 缓存
	moduleLoggers.Store(prefix, newLogger)
	return newLogger
}

func (l *Logger) format(msg string) string {
	if l.prefix != "" {
		return fmt.Sprintf("[%s] %s", l.prefix, msg)
	}
	return msg
}

// Debug 调试日志
func (l *Logger) Debug(format string, args ...any) {
	l.zap.Debugf(l.format(format), args...)
}

// Info 信息日志
func (l *Logger) Info(format string, args ...any) {
	l.zap.Infof(l.format(format), args...)
}

// Warn 警告日志
func (l *Logger) Warn(format string, args ...any) {
	l.zap.Warnf(l.format(format), args...)
}

// Error 错误日志
func (l *Logger) Error(format string, args ...any) {
	l.zap.Errorf(l.format(format), args...)
}

// Sync 刷新日志缓冲
func (l *Logger) Sync() {
	_ = l.zap.Sync()
}

// 便捷函数
func Debug(format string, args ...any) { Get().Debug(format, args...) }
func Info(format string, args ...any)  { Get().Info(format, args...) }
func Warn(format string, args ...any)  { Get().Warn(format, args...) }
func Error(format string, args ...any) { Get().Error(format, args...) }
func Sync()                            { Get().Sync() }
