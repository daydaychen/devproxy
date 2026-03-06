#!/bin/bash

echo "==================================="
echo "Smart-Proxy 交互式应用测试"
echo "==================================="
echo ""

# 测试 1: 简单的交互式 bash 命令
echo "📝 测试 1: 简单交互式命令"
echo "运行: echo 'test' | ./devproxy -- bash -c 'read line; echo \"收到: \$line\"'"
echo ""
echo 'test' | ./devproxy -- bash -c 'read line; echo "收到: $line"'
echo ""
echo "✅ 测试 1 完成"
echo ""

# 测试 2: 非交互式命令 (确保没有破坏原有功能)
echo "📝 测试 2: 非交互式命令"
echo "运行: ./devproxy -- echo 'Hello from devproxy'"
echo ""
./devproxy -- echo 'Hello from devproxy'
echo ""
echo "✅ 测试 2 完成"
echo ""

# 测试 3: 带代理配置的命令
echo "📝 测试 3: 带代理配置的命令"
echo "运行: ./devproxy --verbose -- env | grep PROXY"
echo ""
./devproxy --verbose -- env | grep PROXY
echo ""
echo "✅ 测试 3 完成"
echo ""

echo "==================================="
echo "所有测试完成！"
echo "==================================="
echo ""
echo "💡 手动测试建议："
echo "   运行: ./devproxy -- bash"
echo "   然后在打开的 bash 中输入命令，按 Ctrl+D 退出"
echo ""
echo "   运行: ./devproxy -- vim"
echo "   测试 vim 是否能正常交互，输入 :q 退出"
