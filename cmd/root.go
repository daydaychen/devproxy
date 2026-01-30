package cmd

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"smart-proxy/pkg/process"
	"smart-proxy/pkg/proxy"
	"smart-proxy/pkg/util"

	"github.com/spf13/cobra"
)

var (
	matchPatterns   []string
	overwriteRules  []string
	upstreamProxy   string
	port            int
	verbose         bool
	logFile         string
)

var rootCmd = &cobra.Command{
	Use:   "smart-proxy [flags] -- <command> [args...]",
	Short: "Smart Proxy - 智能MITM代理工具",
	Long: `Smart Proxy 是一个智能的MITM代理工具，可以拦截并修改HTTP/HTTPS请求。
它只代理启动的子进程流量，不影响系统其他进程。

示例:
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
}

func run(cmd *cobra.Command, args []string) {
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

	// 添加URL匹配器
	for _, pattern := range matchPatterns {
		matcher := proxy.NewStringMatcher(pattern)
		proxyServer.AddMatcher(matcher)
	}

	// 添加请求头重写器
	for _, rule := range overwriteRules {
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

		rewriter := proxy.NewHeaderRewriter(headerName, headerValue)
		proxyServer.AddRewriter(rewriter)
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
