// Package logging 创建结构化日志与可选轮转文件。
package logging

import (
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"gopkg.in/natefinch/lumberjack.v2"
)

// Config 描述日志输出与文件轮转配置。
type Config struct {
	Level      string `yaml:"level"`        // 最低日志级别。
	Stdout     bool   `yaml:"stdout"`       // 是否输出到标准输出。
	File       string `yaml:"file"`         // 日志文件路径，空值表示不写文件。
	MaxSizeMB  int    `yaml:"max_size_mb"`  // 单个日志文件的最大大小，单位 MB。
	MaxBackups int    `yaml:"max_backups"`  // 保留的轮转文件数量。
	MaxAgeDays int    `yaml:"max_age_days"` // 轮转文件保留天数。
	Compress   bool   `yaml:"compress"`     // 是否压缩轮转文件。
}

// Validate 校验日志级别、输出位置和轮转参数。
func (c Config) Validate() error {
	var level slog.Level
	if err := level.UnmarshalText([]byte(c.Level)); err != nil {
		return errors.New("logging.level must be debug, info, warn or error")
	}

	if !c.Stdout && c.File == "" {
		return errors.New("logging needs stdout or a file")
	}

	if c.File != "" && (c.MaxSizeMB < 1 || c.MaxSizeMB > 1024 || c.MaxBackups < 1 || c.MaxBackups > 100 || c.MaxAgeDays < 1 || c.MaxAgeDays > 3650) {
		return errors.New("invalid log rotation limits")
	}

	return nil
}

// Open 创建结构化日志器及其关闭函数；多个进程应使用不同文件或标准输出。
func Open(c Config, service string) (*slog.Logger, func() error, error) {
	if err := c.Validate(); err != nil {
		return nil, nil, err
	}

	var level slog.Level
	_ = level.UnmarshalText([]byte(c.Level))
	writers := []io.Writer{}
	if c.Stdout {
		writers = append(writers, os.Stdout)
	}

	closeLog := func() error { return nil }
	if c.File != "" {
		if err := os.MkdirAll(filepath.Dir(c.File), 0750); err != nil {
			return nil, nil, errors.New("cannot create log directory")
		}

		file, err := os.OpenFile(c.File, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0640)
		if err != nil {
			return nil, nil, errors.New("cannot open log file")
		}

		if err = file.Close(); err != nil {
			return nil, nil, err
		}

		rotating := &lumberjack.Logger{
			Filename:   c.File,
			MaxSize:    c.MaxSizeMB,
			MaxBackups: c.MaxBackups,
			MaxAge:     c.MaxAgeDays,
			Compress:   c.Compress,
		}
		writers = append(writers, rotating)
		closeLog = rotating.Close
	}

	return slog.New(slog.NewJSONHandler(io.MultiWriter(writers...), &slog.HandlerOptions{Level: level})).With("service", service), closeLog, nil
}
