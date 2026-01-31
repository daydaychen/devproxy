package cmd

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"smart-proxy/pkg/config"
	"smart-proxy/pkg/process"
	"smart-proxy/pkg/proxy"
	"smart-proxy/pkg/util"

	"github.com/spf13/cobra"
	"golang.org/x/term"
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
	rootCmd.Version = util.Version
	rootCmd.SetVersionTemplate("smart-proxy version {{.Version}}\n")

	rootCmd.Flags().StringArrayVar(&matchPatterns, "match", []string{}, "URL匹配规则 (可指定多次)")
	rootCmd.Flags().StringArrayVar(&overwriteRules, "overwrite", []string{}, "请求头重写规则 (格式: header=value, 可指定多次)")
	rootCmd.Flags().StringVar(&upstreamProxy, "upstream", "", "上游代理地址 (例: http://127.0.0.1:7890)")
	rootCmd.Flags().IntVar(&port, "port", 0, "代理服务器端口 (默认随机分配)")
	rootCmd.Flags().BoolVarP(&verbose, "verbose", "V", false, "详细日志输出")
	rootCmd.Flags().StringVar(&logFile, "log-file", "", "日志文件路径 (用于避免干扰交互式应用，如vim)")
	rootCmd.Flags().StringVarP(&configFile, "config", "c", "", "配置文件路径 (支持 YAML 格式)")
	// 添加 -v 作为 --version 的缩写
	rootCmd.Flags().BoolP("version", "v", false, "查看版本号")
}


func run(cmd *cobra.Command, args []string) {
	// 加载配置文件
	loadConfig(cmd)

	var logFileWriter *os.File
	var loggerInstance *log.Logger = log.Default()

	var origStdout *os.File = os.Stdout
	var origStderr *os.File = os.Stderr

	if logFile != "" {
		var err error
		logFileWriter, err = os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			log.Fatalf("打开日志文件失败: %v", err)
		}
		defer logFileWriter.Close()
		
		fmt.Printf("详细日志将输出到文件: %s (已过滤终端控制符)\n", logFile)
		
		// 创建一个带剥离 ANSI 功能的 Writer
		strippedWriter := &util.AnsiStripper{Writer: logFileWriter}
		// 1. 设置全局默认 Logger 的输出，拦截所有第三方库的 log.Printf
		log.SetOutput(strippedWriter)
		// 2. 创建一个独立的 logger 实例供代理使用
		loggerInstance = log.New(strippedWriter, "", log.LstdFlags)
		
		// 3. 架构级修复：物理劫持底层的 FD 1 和 FD 2
		// 备份原始的终端输出流，供稍后的 PTY 使用
		origStdout, origStderr, err = util.HijackStandardStreams(logFileWriter)
		if err != nil {
			log.Printf("警告: 无法重定向系统标准流: %v", err)
		}

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
	proxyServer := proxy.NewProxyServer(proxyPort, upstreamProxy, verbose, loggerInstance)

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
	// 注意：我们使用劫持前的原始终端流 (origStdout/origStderr) 分配给进程启动器。
	// 这样子进程的 UI 数据会直接写入终端，而父进程及其库产生的日志会被导向 logFile。
	launcher.Stdout = origStdout
	launcher.Stderr = origStderr
	launcher.Stdin = os.Stdin

	// 如果是交互式终端，进入 raw 模式以便子进程完全接管终端
	var oldState *term.State
	if term.IsTerminal(int(os.Stdin.Fd())) {
		var err error
		oldState, err = term.MakeRaw(int(os.Stdin.Fd()))
		if err != nil {
			log.Printf("无法设置终端为 Raw 模式: %v", err)
		} else {
			// 在 Raw 模式下，如果日志直接输出到终端，需要处理 \n 变为 \r\n，否则会出现“阶梯状”缩进
			if logFile == "" {
				log.SetOutput(&util.CrLfFixer{Writer: origStderr})
			}
		}
	}

	// 统一恢复终端的辅助函数
	restoreTerminal := func() {
		if oldState != nil {
			term.Restore(int(os.Stdin.Fd()), oldState)
			// 如果之前切换到了 CrLfFixer，则恢复原始输出
			if logFile == "" {
				log.SetOutput(origStderr)
			}
			// 在恢复终端后，打印一个回车符，确保后续首行日志从第一列开始
			fmt.Print("\r")
			oldState = nil
		}
	}
	// 确保程序退出时恢复终端
	defer restoreTerminal()

	// 启动子进程
	if err := launcher.Start(); err != nil {
		restoreTerminal()
		log.Fatalf("启动子进程失败: %v", err)
	}

	// 处理系统信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 等待信号
	go func() {
		sig := <-sigChan
		restoreTerminal()
		log.Printf("\n收到终止信号 (%v)，正在清理...", sig)
		launcher.Stop()
		proxyServer.Stop()
		if logFileWriter != nil {
			logFileWriter.Sync()
		}
		os.Exit(0)
	}()

	// 等待子进程结束
	err = launcher.Wait()
	// 在打印任何结束日志前先恢复终端，避免 raw 模式导致的缩进问题
	restoreTerminal()

	var exitCode int
	if err != nil {
		log.Printf("子进程运行出错: %v", err)
		if exitError, ok := err.(*exec.ExitError); ok {
			exitCode = exitError.ExitCode()
		} else {
			exitCode = 1
		}
	} else {
		log.Println("子进程正常结束")
	}

	log.Println("正在清理并退出...")
	proxyServer.Stop()

	// 在退出前给 log 库一点时间处理最后的写入
	if logFileWriter != nil {
		log.Println("=== Smart Proxy 结束 ===")
		logFileWriter.Sync()
		// 稍微等待确保 OS 刷写完成
		time.Sleep(100 * time.Millisecond)
	}

	os.Exit(exitCode)
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
