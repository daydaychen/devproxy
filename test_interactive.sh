#!/bin/bash

echo "测试交互式应用支持..."
echo "================================"
echo ""
echo "测试1: 运行一个简单的交互式shell命令"
echo "命令: ./devproxy -- bash -c 'echo \"Hello from proxied bash\"; read -p \"输入任意内容: \" input; echo \"你输入了: \$input\"'"
echo ""
echo "请在提示时输入一些文字，然后按回车"
echo ""

./devproxy -- bash -c 'echo "Hello from proxied bash"; read -p "输入任意内容: " input; echo "你输入了: $input"'

echo ""
echo "================================"
echo "测试完成!"
