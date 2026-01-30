#!/bin/bash

echo "==================================="
echo "Smart-Proxy 交互式应用测试"
echo "==================================="
echo ""

# 测试 1: 简单的交互式 bash 命令
echo "📝 测试 1: 简单交互式命令"
echo "运行: echo 'test' | ./smart-proxy -- bash -c 'read line; echo \"收到: \$line\"'"
echo ""
echo 'test' | ./smart-proxy -- bash -c 'read line; echo "收到: $line"'
echo ""
echo "✅ 测试 1 完成"
echo ""

# 测试 2: 非交互式命令 (确保没有破坏原有功能)
echo "📝 测试 2: 非交互式命令"
echo "运行: ./smart-proxy -- echo 'Hello from smart-proxy'"
echo ""
./smart-proxy -- echo 'Hello from smart-proxy'
echo ""
echo "✅ 测试 2 完成"
echo ""

# 测试 3: 带代理配置的命令
echo "📝 测试 3: 带代理配置的命令"
echo "运行: ./smart-proxy --verbose -- env | grep PROXY"
echo ""
./smart-proxy --verbose -- env | grep PROXY
echo ""
echo "✅ 测试 3 完成"
echo ""

echo "==================================="
echo "所有测试完成！"
echo "==================================="
echo ""
echo "💡 手动测试建议："
echo "   运行: ./smart-proxy -- bash"
echo "   然后在打开的 bash 中输入命令，按 Ctrl+D 退出"
echo ""
echo "   运行: ./smart-proxy -- vim"
echo "   测试 vim 是否能正常交互，输入 :q 退出"
