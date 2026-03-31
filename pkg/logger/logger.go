package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// Level 日志级别
type Level string

const (
	DEBUG Level = "DEBUG"
	INFO  Level = "INFO"
	WARN  Level = "WARN"
	ERROR Level = "ERROR"
)

// Logger 日志器
type Logger struct {
	mu             sync.Mutex
	baseDir        string        // 日志根目录
	serviceName    string        // 服务名(用于子目录)
	maxAge         time.Duration // 保留时长
	currentFile    *os.File
	currentDate    string // "2026-03-22"
	currentHour     int    // 20
	stopCh         chan struct{}
	doneCh         chan struct{}
	writer         io.Writer // 可选的外部writer(如控制台)
	rotateCheckCh  chan struct{} // 异步轮转检查通道
	needRotate     atomic.Bool   // 需要轮转标志
}

// Config 日志配置
type Config struct {
	BaseDir     string        // 日志根目录
	ServiceName string        // 服务名
	MaxAge      time.Duration // 保留时长
	Writer      io.Writer     // 额外输出(如控制台)
}

// NewLogger 创建日志器
func NewLogger(cfg *Config) (*Logger, error) {
	if cfg.MaxAge == 0 {
		cfg.MaxAge = 7 * 24 * time.Hour
	}

	l := &Logger{
		baseDir:       cfg.BaseDir,
		serviceName:   cfg.ServiceName,
		maxAge:        cfg.MaxAge,
		stopCh:        make(chan struct{}),
		doneCh:        make(chan struct{}),
		writer:        cfg.Writer,
		rotateCheckCh: make(chan struct{}, 1),
	}

	// 初始创建文件
	if err := l.rotateIfNeeded(); err != nil {
		return nil, fmt.Errorf("failed to create log file: %w", err)
	}

	// 启动清理goroutine
	go l.cleanupLoop()

	// 启动异步轮转检查
	go l.rotateCheckLoop()

	return l, nil
}

// rotateIfNeeded 检查并切换日志文件
func (l *Logger) rotateIfNeeded() error {
	now := time.Now()
	date := now.Format("2006-01-02")
	hour := now.Hour()

	// 检查是否需要切换
	if l.currentFile != nil && l.currentDate == date && l.currentHour == hour {
		return nil
	}

	// 关闭当前文件
	if l.currentFile != nil {
		l.currentFile.Close()
	}

	// 创建目录
	dirPath := filepath.Join(l.baseDir, l.serviceName, date)
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	// 创建新文件
	fileName := fmt.Sprintf("raw-%s-%02d.log", date, hour)
	filePath := filepath.Join(dirPath, fileName)

	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}

	l.currentFile = f
	l.currentDate = date
	l.currentHour = hour
	l.needRotate.Store(false)

	return nil
}

// rotateCheckLoop 异步轮转检查循环
func (l *Logger) rotateCheckLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			l.checkAndMarkRotate()
		case <-l.rotateCheckCh:
			l.checkAndMarkRotate()
		case <-l.stopCh:
			return
		}
	}
}

// checkAndMarkRotate 检查是否需要轮转并标记
func (l *Logger) checkAndMarkRotate() {
	now := time.Now()
	date := now.Format("2006-01-02")
	hour := now.Hour()

	l.mu.Lock()
	needRotate := l.currentDate != date || l.currentHour != hour
	l.mu.Unlock()

	if needRotate {
		l.needRotate.Store(true)
	}
}

// write 写入日志
func (l *Logger) write(level Level, msg string, fields []Field) {
	// 快速检查是否需要轮转（无锁）
	if l.needRotate.Load() {
		l.mu.Lock()
		if err := l.rotateIfNeeded(); err != nil {
			l.mu.Unlock()
			fmt.Fprintf(os.Stderr, "[LOGGER ERROR] %v\n", err)
			return
		}
		l.mu.Unlock()
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	// 格式化时间
	now := time.Now()
	timestamp := now.Format("2006-01-02T15:04:05.000Z07:00")

	// 构建日志行
	var line string
	if len(fields) > 0 {
		fieldStrs := make([]string, len(fields))
		for i, f := range fields {
			fieldStrs[i] = fmt.Sprintf("%s=%s", f.Key, formatValue(f.Value))
		}
		line = fmt.Sprintf("%s %-5s %s %s\n", timestamp, level, msg, joinFields(fieldStrs))
	} else {
		line = fmt.Sprintf("%s %-5s %s\n", timestamp, level, msg)
	}

	// 写入文件
	if l.currentFile != nil {
		if _, err := l.currentFile.WriteString(line); err != nil {
			fmt.Fprintf(os.Stderr, "[LOGGER ERROR] write failed: %v\n", err)
		}
	}

	// 写入额外输出(如控制台)
	if l.writer != nil {
		l.writer.Write([]byte(line))
	}
}

// joinFields 连接字段
func joinFields(fields []string) string {
	result := ""
	for _, f := range fields {
		result += f + " "
	}
	return result
}

// Debug 记录DEBUG日志
func (l *Logger) Debug(msg string, fields ...Field) {
	l.write(DEBUG, msg, fields)
}

// Info 记录INFO日志
func (l *Logger) Info(msg string, fields ...Field) {
	l.write(INFO, msg, fields)
}

// Warn 记录WARN日志
func (l *Logger) Warn(msg string, fields ...Field) {
	l.write(WARN, msg, fields)
}

// Error 记录ERROR日志
func (l *Logger) Error(msg string, fields ...Field) {
	l.write(ERROR, msg, fields)
}

// cleanupLoop 定期清理过期日志
func (l *Logger) cleanupLoop() {
	// 启动时先清理一次
	l.cleanOldLogs()

	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			l.cleanOldLogs()
		case <-l.stopCh:
			close(l.doneCh)
			return
		}
	}
}

// cleanOldLogs 清理过期日志
func (l *Logger) cleanOldLogs() {
	serviceDir := filepath.Join(l.baseDir, l.serviceName)

	// 检查目录是否存在
	if _, err := os.Stat(serviceDir); os.IsNotExist(err) {
		return
	}

	// 读取所有日期目录
	entries, err := os.ReadDir(serviceDir)
	if err != nil {
		return
	}

	cutoff := time.Now().Add(-l.maxAge)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// 解析目录名(日期格式 2006-01-02)
		dirTime, err := time.Parse("2006-01-02", entry.Name())
		if err != nil {
			continue
		}

		// 删除过期目录
		if dirTime.Before(cutoff) {
			dirPath := filepath.Join(serviceDir, entry.Name())
			os.RemoveAll(dirPath)
		}
	}
}

// Close 关闭日志器
func (l *Logger) Close() error {
	close(l.stopCh)
	<-l.doneCh

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.currentFile != nil {
		return l.currentFile.Close()
	}
	return nil
}

// Sync 刷新缓冲
func (l *Logger) Sync() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.currentFile != nil {
		return l.currentFile.Sync()
	}
	return nil
}
