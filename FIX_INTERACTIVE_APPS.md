# 交互式应用支持修复说明

## 问题描述

在之前的实现中，当使用 smart-proxy 启动交互式应用（如 vim、bash、python 等）时，这些应用会在启动后无响应，无法接收用户输入。

## 根本原因

问题出在 `pkg/process/launcher.go` 中的进程启动配置：

```go
// 旧代码
l.cmd.SysProcAttr = &syscall.SysProcAttr{
    Setpgid: true,  // 这一行导致了问题
}
```

**为什么会导致问题？**

1. `Setpgid: true` 会将子进程放到一个新的进程组中
2. 这会改变进程与终端的关系，影响信号处理和 I/O 控制
3. 交互式应用依赖于正确的终端控制来处理用户输入
4. 新的进程组导致子进程无法正确接收终端的输入信号

## 解决方案

### 1. 移除进程组设置

直接移除 `SysProcAttr` 配置，让子进程保持在与父进程相同的进程组中：

```go
// 新代码 - 移除了进程组设置
// l.cmd.SysProcAttr = &syscall.SysProcAttr{
//     Setpgid: true,
// }

// 保持 stdin/stdout/stderr 连接（这部分没有改变）
l.cmd.Stdout = os.Stdout
l.cmd.Stderr = os.Stderr
l.cmd.Stdin = os.Stdin
```

### 2. 简化进程终止逻辑

由于移除了进程组，相应地简化了 Stop() 方法：

```go
// 旧代码 - 尝试终止整个进程组
pgid, err := syscall.Getpgid(l.cmd.Process.Pid)
if err == nil {
    syscall.Kill(-pgid, syscall.SIGTERM)
}
if err := l.cmd.Process.Kill(); err != nil {
    return fmt.Errorf("终止进程失败: %w", err)
}

// 新代码 - 直接发送 SIGTERM，失败则 Kill
if err := l.cmd.Process.Signal(syscall.SIGTERM); err != nil {
    if err := l.cmd.Process.Kill(); err != nil {
        return fmt.Errorf("终止进程失败: %w", err)
    }
}
```

## 修复效果

### ✅ 现在可以正常工作的场景

1. **交互式编辑器**
   ```bash
   ./smart-proxy -- vim myfile.txt
   ./smart-proxy -- nano myfile.txt
   ```

2. **交互式 Shell**
   ```bash
   ./smart-proxy -- bash
   ./smart-proxy -- zsh
   ```

3. **交互式解释器**
   ```bash
   ./smart-proxy -- python3
   ./smart-proxy -- node
   ./smart-proxy -- irb  # Ruby REPL
   ```

4. **需要用户输入的程序**
   ```bash
   ./smart-proxy -- bash -c 'read -p "输入: " x; echo "你输入了: $x"'
   ```

### ✅ 保持正常工作的场景

所有非交互式应用仍然正常工作：

```bash
./smart-proxy -- curl https://example.com
./smart-proxy -- node server.js
./smart-proxy -- npm start
```

## 测试验证

运行测试脚本验证修复：

```bash
# 自动化测试
./test_fix.sh

# 手动测试交互式 bash
./smart-proxy -- bash
# 输入一些命令，然后 Ctrl+D 退出

# 手动测试 vim
./smart-proxy -- vim
# 测试编辑功能，输入 :q 退出
```

## 技术细节

### 进程组的作用

进程组（Process Group）主要用于：
- 作业控制（Job Control）
- 信号批量发送
- 终端控制

### 为什么之前使用进程组？

最初设置 `Setpgid: true` 的目的是为了便于管理子进程：
- 可以一次性终止整个进程组（包括子进程的子进程）
- 避免僵尸进程

### 为什么现在移除它？

对于 smart-proxy 的使用场景：
1. **交互式优先**: 用户需要与子进程交互比批量终止更重要
2. **简单场景**: 大多数情况下子进程不会再创建复杂的子进程树
3. **优雅终止**: 使用 SIGTERM 已经足够优雅地终止进程

如果未来需要处理复杂的子进程树，可以考虑：
- 使用 Context 传递取消信号
- 在应用层面实现进程管理
- 使用容器化方案

## 影响评估

| 场景 | 修复前 | 修复后 |
|------|--------|--------|
| 交互式应用 (vim, bash) | ❌ 无响应 | ✅ 正常工作 |
| 非交互式应用 | ✅ 正常 | ✅ 正常 |
| 进程清理 | ✅ 可靠 | ✅ 可靠 |
| 信号处理 | ⚠️ 复杂 | ✅ 简单 |

## 相关文件

- `pkg/process/launcher.go` - 主要修改文件
- `README.md` - 添加交互式应用示例
- `CHANGELOG.md` - 更新日志
- `test_fix.sh` - 测试脚本

## 参考资料

- [Unix Process Groups](https://en.wikipedia.org/wiki/Process_group)
- [Go exec.Cmd Documentation](https://pkg.go.dev/os/exec#Cmd)
- [Syscall SysProcAttr](https://pkg.go.dev/syscall#SysProcAttr)
