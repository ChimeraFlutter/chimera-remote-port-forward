package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/chimera/chimera-remote-port-forward/internal/client"
	"github.com/chimera/chimera-remote-port-forward/internal/server"
	"github.com/chimera/chimera-remote-port-forward/pkg/config"
	"github.com/chimera/chimera-remote-port-forward/pkg/logger"
)

func main() {
	// 通用参数
	mode := flag.String("mode", "", "运行模式: server 或 client (必需)")
	configFile := flag.String("config", "", "配置文件路径 (可选)")

	// 服务端参数
	listen := flag.String("listen", "", "WebSocket监听地址 (默认 :52341)")
	web := flag.String("web", "", "Web界面地址 (默认 :52342)")
	portStart := flag.Int("port-start", 0, "端口范围起始 (默认 10000)")
	portEnd := flag.Int("port-end", 0, "端口范围结束 (默认 11000)")
	token := flag.String("token", "", "认证Token")
	webPassword := flag.String("web-password", "", "Web界面密码 (默认 admin)")
	logDir := flag.String("log-dir", "", "日志目录 (默认 C:/logs/chimera-remote-port-forward)")
	logMaxAge := flag.Int("log-max-age", 0, "日志保留天数 (默认 7)")

	// 客户端参数
	serverAddr := flag.String("server", "", "服务端地址 (例如 ws://localhost:52341/ws)")
	deviceName := flag.String("device-name", "", "设备名称")
	localIP := flag.String("local-ip", "", "本地IP (默认 127.0.0.1)")
	localPort := flag.Int("local-port", 0, "本地端口")

	flag.Parse()

	// 验证mode参数
	if *mode == "" {
		log.Fatal("[ERROR] 必须指定 -mode 参数: server 或 client")
	}

	// 根据mode启动对应服务
	switch *mode {
	case "server":
		startServer(*configFile, *listen, *web, *portStart, *portEnd, *token, *webPassword, *logDir, *logMaxAge)
	case "client":
		startClient(*configFile, *serverAddr, *deviceName, *localIP, *localPort, *token, *logDir, *logMaxAge)
	default:
		log.Fatalf("[ERROR] 无效的mode: %s, 必须是 server 或 client", *mode)
	}
}

func startServer(configFile, listen, web string, portStart, portEnd int, token, webPassword, logDir string, logMaxAge int) {
	// 加载默认配置
	cfg := config.DefaultServerConfig()

	// 如果提供了配置文件，加载并覆盖默认配置
	if configFile != "" {
		fileCfg, err := config.LoadServerConfig(configFile)
		if err != nil {
			log.Fatalf("[ERROR] Failed to load config file: %v", err)
		}
		// 合并配置文件设置
		if fileCfg.Listen != "" {
			cfg.Listen = fileCfg.Listen
		}
		if fileCfg.Web != "" {
			cfg.Web = fileCfg.Web
		}
		if fileCfg.PortStart > 0 {
			cfg.PortStart = fileCfg.PortStart
		}
		if fileCfg.PortEnd > 0 {
			cfg.PortEnd = fileCfg.PortEnd
		}
		if fileCfg.AuthToken != "" {
			cfg.AuthToken = fileCfg.AuthToken
		}
		if fileCfg.WebPassword != "" {
			cfg.WebPassword = fileCfg.WebPassword
		}
		if fileCfg.LogDir != "" {
			cfg.LogDir = fileCfg.LogDir
		}
		if fileCfg.LogMaxAge > 0 {
			cfg.LogMaxAge = fileCfg.LogMaxAge
		}
		if fileCfg.HeartbeatTimeout > 0 {
			cfg.HeartbeatTimeout = fileCfg.HeartbeatTimeout
		}
		if fileCfg.MaxDevices > 0 {
			cfg.MaxDevices = fileCfg.MaxDevices
		}
		if fileCfg.MaxConnsPerProxy > 0 {
			cfg.MaxConnsPerProxy = fileCfg.MaxConnsPerProxy
		}
	}

	// 命令行参数覆盖
	if listen != "" {
		cfg.Listen = listen
	}
	if web != "" {
		cfg.Web = web
	}
	if portStart > 0 {
		cfg.PortStart = portStart
	}
	if portEnd > 0 {
		cfg.PortEnd = portEnd
	}
	if token != "" {
		cfg.AuthToken = token
	}
	if webPassword != "" {
		cfg.WebPassword = webPassword
	}
	if logDir != "" {
		cfg.LogDir = logDir
	}
	if logMaxAge > 0 {
		cfg.LogMaxAge = logMaxAge
	}

	// 初始化日志
	logLogger, err := logger.NewLogger(&logger.Config{
		BaseDir:     cfg.LogDir,
		ServiceName: "server",
		MaxAge:      time.Duration(cfg.LogMaxAge) * 24 * time.Hour,
		Writer:      os.Stdout,
	})
	if err != nil {
		log.Fatalf("[ERROR] Failed to init logger: %v", err)
	}

	// 创建并启动服务端
	s := server.NewServer(cfg, logLogger)

	// 处理信号
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		logLogger.Info("Shutting down")
		s.Stop()
		logLogger.Info("Server stopped, closing logger")

		// 使用 goroutine 关闭 logger，带超时
		done := make(chan struct{})
		go func() {
			logLogger.Close()
			close(done)
		}()

		select {
		case <-done:
		case <-time.After(3 * time.Second):
			logLogger.Info("Logger close timeout, exiting anyway")
		}

		os.Exit(0)
	}()

	logLogger.Info("Starting server")
	if err := s.Start(); err != nil {
		logLogger.Error("Server failed", logger.Err(err))
		logLogger.Close()
		os.Exit(1)
	}
}

func startClient(configFile, serverAddr, deviceName, localIP string, localPort int, token, logDir string, logMaxAge int) {
	// 加载默认配置
	cfg := config.DefaultClientConfig()

	// 如果提供了配置文件，加载并覆盖默认配置
	if configFile != "" {
		fileCfg, err := config.LoadClientConfig(configFile)
		if err != nil {
			log.Fatalf("[ERROR] Failed to load config file: %v", err)
		}
		// 合并配置文件设置
		if fileCfg.Server != "" {
			cfg.Server = fileCfg.Server
		}
		if fileCfg.DeviceName != "" {
			cfg.DeviceName = fileCfg.DeviceName
		}
		if fileCfg.LocalIP != "" {
			cfg.LocalIP = fileCfg.LocalIP
		}
		if fileCfg.LocalPort > 0 {
			cfg.LocalPort = fileCfg.LocalPort
		}
		if fileCfg.Token != "" {
			cfg.Token = fileCfg.Token
		}
		if fileCfg.LogDir != "" {
			cfg.LogDir = fileCfg.LogDir
		}
		if fileCfg.LogMaxAge > 0 {
			cfg.LogMaxAge = fileCfg.LogMaxAge
		}
		if fileCfg.HeartbeatInterval > 0 {
			cfg.HeartbeatInterval = fileCfg.HeartbeatInterval
		}
		if fileCfg.ReconnectInterval > 0 {
			cfg.ReconnectInterval = fileCfg.ReconnectInterval
		}
	}

	// 命令行参数覆盖
	if serverAddr != "" {
		cfg.Server = serverAddr
	}
	if deviceName != "" {
		cfg.DeviceName = deviceName
	}
	if localIP != "" {
		cfg.LocalIP = localIP
	}
	if localPort > 0 {
		cfg.LocalPort = localPort
	}
	if token != "" {
		cfg.Token = token
	}
	if logDir != "" {
		cfg.LogDir = logDir
	}
	if logMaxAge > 0 {
		cfg.LogMaxAge = logMaxAge
	}

	// 验证必要参数
	if cfg.Server == "" {
		log.Fatal("[ERROR] 必须指定 -server 参数")
	}
	if cfg.DeviceName == "" {
		log.Fatal("[ERROR] 必须指定 -device-name 参数")
	}
	if cfg.LocalPort <= 0 {
		log.Fatal("[ERROR] 必须指定 -local-port 参数")
	}

	// 初始化日志
	logLogger, err := logger.NewLogger(&logger.Config{
		BaseDir:     cfg.LogDir,
		ServiceName: "client",
		MaxAge:      time.Duration(cfg.LogMaxAge) * 24 * time.Hour,
		Writer:      os.Stdout,
	})
	if err != nil {
		log.Fatalf("[ERROR] Failed to init logger: %v", err)
	}

	// 创建客户端
	c := client.NewClient(cfg, logLogger)

	// 处理信号
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		logLogger.Info("Shutting down")
		c.Stop()
		logLogger.Close()
		os.Exit(0)
	}()

	logLogger.Info("Starting client")
	if err := c.Start(); err != nil {
		logLogger.Error("Client failed", logger.Err(err))
		logLogger.Close()
		os.Exit(1)
	}
}
