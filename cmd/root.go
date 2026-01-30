package cmd

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"smart-proxy/pkg/config"
	"smart-proxy/pkg/process"
	"smart-proxy/pkg/proxy"
	"smart-proxy/pkg/util"

	"github.com/spf13/cobra"
)

func loadConfig(cmd *cobra.Command) {
	// 收集所有配置源
	var configFiles []string

	// 1. 全局配置 (~/.config/smart-proxy/global.yaml)
	home, err := os.UserHomeDir()
	if err == nil {
		globalPath := fmt.Sprintf("%s/.config/smart-proxy/global.yaml", home)
		if _, err := os.Stat(globalPath); err == nil {
			configFiles = append(configFiles, globalPath)
		}
	}

	// 2. 目录级别配置 (优先级高于全局)
	// 如果命令行指定了 --config，则只使用该文件作为“目录级/显式”配置
	if configFile != "" {
		configFiles = append(configFiles, configFile)
	} else {
		// 检查默认本地配置文件
		defaults := []string{"smart-proxy.yaml", ".smart-proxy.yaml"}
		for _, d := range defaults {
			if _, err := os.Stat(d); err == nil {
				configFiles = append(configFiles, d)
				break
			}
		}
	}

	// 按顺序加载并合并
	for _, path := range configFiles {
		cfg, err := config.LoadConfig(path)
		if err != nil {
			// 如果是显式指定的配置文件加载失败，则报错退出
			if configFile != "" && path == configFile {
				log.Fatalf("加载指定的配置文件失败: %v", err)
			}
			// 否则仅打印警告
			fmt.Printf("警告: 加载配置文件 %s 失败: %v\n", path, err)
			continue
		}

		fmt.Printf("集成配置文件: %s\n", path)

		// 合并单值配置 (如果命令行没指定，后面的文件会覆盖前面的)
		if !cmd.Flags().Changed("upstream") && cfg.Upstream != "" {
			upstreamProxy = cfg.Upstream
		}
		if !cmd.Flags().Changed("port") && cfg.Port > 0 {
			port = cfg.Port
		}
		if !cmd.Flags().Changed("verbose") {
			// 如果配置文件中明确设置了 verbose，则应用它
			// 注意：这里默认是 false，所以我们只在配置文件为 true 时更新
			if cfg.Verbose {
				verbose = true
			}
		}
		if !cmd.Flags().Changed("log-file") && cfg.LogFile != "" {
			logFile = cfg.LogFile
		}

		// 合并列表配置 (累加)
		if len(cfg.Match) > 0 {
			matchPatterns = append(matchPatterns, cfg.Match...)
		}
		if len(cfg.Overwrite) > 0 {
			for k, v := range cfg.Overwrite {
				overwriteRules = append(overwriteRules, fmt.Sprintf("%s=%s", k, v))
			}
		}
		if len(cfg.Rules) > 0 {
			configRules = append(configRules, cfg.Rules...)
		}
	}
}

var (
	matchPatterns  []string
	overwriteRules []string
	configRules    []config.RuleConfig
	upstreamProxy  string
	port           int
	verbose        bool
	logFile        string
	configFile     string
)

var rootCmd = &cobra.Command{
	Use:   "smart-proxy [flags] -- <command> [args...]",
	Short: "Smart Proxy - 智能MITM代理工具",
	Long: `Smart Proxy 是一个智能的MITM代理工具，可以拦截并修改HTTP/HTTPS请求。
它只代理启动的子进程流量，不影响系统其他进程。

示例:
  # 使用配置文件
  smart-proxy --config config.yaml -- node server.js

  # 基本用法
  smart-proxy --match "domain.com/v1/api" --overwrite useragent=CustomUA -- node server.js

  # 带上游代理
  smart-proxy --match "*.example.com" --overwrite useragent=Bot --upstream http://127.0.0.1:7890 -- node app.js

  # 指定端口和详细日志
  smart-proxy --port 8888 --match "/api/" --overwrite useragent=Test --verbose -- npm start

  # 运行交互式应用（如 vim）时，建议将 verbose 日志输出到文件
  smart-proxy --verbose --log-file ./proxy.log -- vim test.js`,
	Args: cobra.MinimumNArgs(1),
	Run:  run,
}

func init() {
	rootCmd.Flags().StringArrayVar(&matchPatterns, "match", []string{}, "URL匹配规则 (可指定多次)")
	rootCmd.Flags().StringArrayVar(&overwriteRules, "overwrite", []string{}, "请求头重写规则 (格式: header=value, 可指定多次)")
	rootCmd.Flags().StringVar(&upstreamProxy, "upstream", "", "上游代理地址 (例: http://127.0.0.1:7890)")
	rootCmd.Flags().IntVar(&port, "port", 0, "代理服务器端口 (默认随机分配)")
	rootCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "详细日志输出")
	rootCmd.Flags().StringVar(&logFile, "log-file", "", "日志文件路径 (用于避免干扰交互式应用，如vim)")
	rootCmd.Flags().StringVarP(&configFile, "config", "c", "", "配置文件路径 (支持 YAML 格式)")
}

func run(cmd *cobra.Command, args []string) {
	// 加载配置文件
	loadConfig(cmd)

	// 设置日志输出
	if logFile != "" {
		f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			log.Fatalf("打开日志文件失败: %v", err)
		}
		defer f.Close()
		
		fmt.Printf("详细日志将输出到文件: %s\n", logFile)
		log.SetOutput(f)
		log.Printf("=== Smart Proxy 启动 (PID: %d) ===", os.Getpid())
	}

	// 分配端口
	var proxyPort int
	var err error
	if port > 0 {
		proxyPort = port
	} else {
		proxyPort, err = util.GetRandomPort()
		if err != nil {
			log.Fatalf("分配随机端口失败: %v", err)
		}
	}

	// 创建代理服务器
	proxyServer := proxy.NewProxyServer(proxyPort, upstreamProxy, verbose)

	// 1. 处理默认规则 (命令行参数 + 配置文件顶层规则)
	for _, pattern := range matchPatterns {
		proxyServer.AddMatcher(proxy.NewStringMatcher(pattern))
	}
	for _, rule := range overwriteRules {
		headerName, headerValue := parseOverwriteRule(rule)
		proxyServer.AddRewriter(proxy.NewHeaderRewriter(headerName, headerValue))
	}

	// 2. 处理成组的规则
	for _, ruleCfg := range configRules {
		ruleName := ruleCfg.Name
		if ruleName == "" {
			ruleName = "named-rule"
		}
		pRule := &proxy.ProxyRule{Name: ruleName}
		
		for _, pattern := range ruleCfg.Match {
			pRule.Matchers = append(pRule.Matchers, proxy.NewStringMatcher(pattern))
		}
		for k, v := range ruleCfg.Overwrite {
			headerName, headerValue := parseOverwriteRule(fmt.Sprintf("%s=%s", k, v))
			pRule.Rewriters = append(pRule.Rewriters, proxy.NewHeaderRewriter(headerName, headerValue))
		}
		proxyServer.AddRule(pRule)
	}

	// 启动代理服务器
	if err := proxyServer.Start(); err != nil {
		log.Fatalf("启动代理服务器失败: %v", err)
	}
	defer proxyServer.Stop()

	// 创建进程启动器
	targetCommand := args[0]
	targetArgs := args[1:]
	launcher := process.NewProcessLauncher(targetCommand, targetArgs, proxyPort, verbose)

	// 启动子进程
	if err := launcher.Start(); err != nil {
		log.Fatalf("启动子进程失败: %v", err)
	}

	// 处理系统信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 等待信号或进程结束
	go func() {
		<-sigChan
		log.Println("\n收到终止信号，正在清理...")
		launcher.Stop()
		proxyServer.Stop()
		os.Exit(0)
	}()

	// 等待子进程结束
	if err := launcher.Wait(); err != nil {
		log.Printf("子进程退出: %v", err)
	}

	log.Println("子进程已结束，正在清理...")
}

// Execute 执行命令
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func parseOverwriteRule(rule string) (string, string) {
	parts := strings.SplitN(rule, "=", 2)
	if len(parts) != 2 {
		log.Fatalf("无效的重写规则: %s (格式应为 header=value)", rule)
	}

	headerName := parts[0]
	headerValue := parts[1]

	// 处理常见的简写
	switch strings.ToLower(headerName) {
	case "useragent", "ua":
		headerName = "User-Agent"
	case "referer":
		headerName = "Referer"
	case "origin":
		headerName = "Origin"
	}

	return headerName, headerValue
}
