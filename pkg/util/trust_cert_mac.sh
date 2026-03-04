#!/bin/bash
# install_proxy_ca.sh

set -e

echo "=== 安装 DevProxy 根证书到 macOS Keychain ==="

# 准备根证书临时文件
CA_FILE="/tmp/devproxy-ca-system.pem"

cat << 'EOF' > "$CA_FILE"
-----BEGIN CERTIFICATE-----
MIIDoTCCAomgAwIBAgIRAPT0C1R7Z8+F038kQfB47vQwDQYJKoZIhvcNAQELBQAw
DjEMMAoGA1UEAxMDbWl0bTAeFw0xNzEwMjkxNjU4MTVaFw0yNzEwMjcxNjU4MTVa
MA4xDDAKBgNVBAMTA21pdG0wggEiMA0GCSqGSIb3DQEBAQUAA4IBDwAwggEKAoIB
AQC8aG5U8C2R2hXqQ2d9m4C6rN/c2sC4y5pM+0Gg18s24pZVqjK2iV0sV1P7y8h8
wH+Lp6X6k9/Zf8qQ9l5zO/v6l6d9L2JgS1MhAWeaRxJwXG3NpHUfXU5p9o9jW4A4
d6JvF0eOa+/P7gN1Xl0Ew5nUoQzUaZmVcR9fL5M1zK7b9H8X5B4G5e2j9Q3rA9C6
D9g3yO6a1F1nQ9U0C+U3K0H9jV5D9J/5vY3V3eK9+M1P+9Y7R1gW1wRzQ3+R9B=
... 此处为 goproxy/goproxy_ca.pem 完整内容，实际应当提取 goproxy.CA_CERT 源码里的证书 ...
EOF

# 提取源码中的实际 CA_CERT
cat << 'EOF' > /tmp/extract_ca.go
package main

import (
	"fmt"
	"os"
	"github.com/elazarl/goproxy"
)

func main() {
    err := os.WriteFile("/tmp/devproxy-ca-system.pem", goproxy.CA_CERT, 0644)
    if err != nil {
        fmt.Println("Error:", err)
    }
}
EOF
go run /tmp/extract_ca.go

echo "1. 已生成根证书： $CA_FILE"
echo "2. 正在尝试将其导入 macOS 系统 keychain 并设置为始终信任..."
echo "需要输入系统密码进行 sudo 授权："

sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain "$CA_FILE"

echo "=== 安装完成 ==="
echo "清理临时文件..."
rm "$CA_FILE" /tmp/extract_ca.go
echo "之后 native-tls / macOS 系统级别的 HTTPS 拦截即可生效。"
