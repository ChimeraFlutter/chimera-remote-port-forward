package config

import (
	"os"
	"path/filepath"
	"runtime"
	"time"

	"gopkg.in/yaml.v3"
)

// ServerConfig 服务端配置
type ServerConfig struct {
	Listen           string        `yaml:"listen" json:"listen"`                       // WebSocket监听地址
	Web              string        `yaml:"web" json:"web"`                             // Web界面地址
	PortStart        int           `yaml:"port_start" json:"port_start"`               // 端口范围起始
	PortEnd          int           `yaml:"port_end" json:"port_end"`                   // 端口范围结束
	HeartbeatTimeout time.Duration `yaml:"heartbeat_timeout" json:"heartbeat_timeout"` // 心跳超时
	AuthToken        string        `yaml:"auth_token" json:"auth_token"`               // 认证Token
	WebPassword      string        `yaml:"web_password" json:"web_password"`           // Web界面密码
	LogDir           string        `yaml:"log_dir" json:"log_dir"`                     // 日志目录
	LogMaxAge        int           `yaml:"log_max_age" json:"log_max_age"`             // 日志保留天数
	MaxDevices       int           `yaml:"max_devices" json:"max_devices"`             // 最大设备连接数
	MaxConnsPerProxy int           `yaml:"max_conns_per_proxy" json:"max_conns_per_proxy"` // 每个代理最大TCP连接数
}

// defaultLogDir 返回跨平台默认日志目录
func defaultLogDir() string {
	switch runtime.GOOS {
	case "windows":
		return "C:/logs/chimera-remote-port-forward"
	case "darwin", "linux":
		// 尝试使用 /var/log，如果不可写则使用用户目录
		logDir := "/var/log/chimera-remote-port-forward"
		if err := os.MkdirAll(logDir, 0755); err == nil {
			return logDir
		}
		// 回退到用户目录
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, ".local", "share", "chimera-remote-port-forward", "logs")
		}
		return "./logs"
	case "android":
		return "/data/local/tmp/chimera-remote-port-forward/logs"
	case "ios":
		// iOS 使用应用沙盒目录
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, "Library", "Logs", "chimera-remote-port-forward")
		}
		return "./logs"
	default:
		return "./logs"
	}
}

// DefaultServerConfig 返回默认服务端配置
func DefaultServerConfig() *ServerConfig {
	return &ServerConfig{
		Listen:           ":52341",
		Web:              ":52342",
		PortStart:        10000,
		PortEnd:          11000,
		HeartbeatTimeout: 90 * time.Second,
		AuthToken:        "",
		WebPassword:      "admin",
		LogDir:           defaultLogDir(),
		LogMaxAge:        7,
		MaxDevices:       1000,
		MaxConnsPerProxy: 10000,
	}
}

// ClientConfig 客户端配置
type ClientConfig struct {
	Server            string        `yaml:"server" json:"server"`                         // 服务端地址
	DeviceName        string        `yaml:"device_name" json:"device_name"`               // 设备名称
	LocalPort         int           `yaml:"local_port" json:"local_port"`                 // 本地端口
	Token             string        `yaml:"token" json:"token"`                           // 认证Token
	HeartbeatInterval time.Duration `yaml:"heartbeat_interval" json:"heartbeat_interval"` // 心跳间隔
	ReconnectInterval time.Duration `yaml:"reconnect_interval" json:"reconnect_interval"` // 重连间隔
	LogDir            string        `yaml:"log_dir" json:"log_dir"`                       // 日志目录
	LogMaxAge         int           `yaml:"log_max_age" json:"log_max_age"`               // 日志保留天数
}

// DefaultClientConfig 返回默认客户端配置
func DefaultClientConfig() *ClientConfig {
	return &ClientConfig{
		HeartbeatInterval: 30 * time.Second,
		ReconnectInterval: 3 * time.Second,
		LogDir:            defaultLogDir(),
		LogMaxAge:         7,
	}
}

// LoadServerConfig 从 YAML 文件加载服务端配置
func LoadServerConfig(path string) (*ServerConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := &ServerConfig{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// LoadClientConfig 从 YAML 文件加载客户端配置
func LoadClientConfig(path string) (*ClientConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := &ClientConfig{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}
