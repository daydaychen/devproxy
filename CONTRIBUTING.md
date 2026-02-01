# 贡献指南

感谢你考虑为 devproxy 贡献代码！本指南将帮助你了解如何参与项目贡献。

## 如何贡献

### 报告问题

如果你发现 bug 或有新功能建议，请先搜索 [issues](https://github.com/yourusername/devproxy/issues) 确保问题未被报告。然后使用 issue 模板提交详细的报告，包括：

- 问题描述
- 复现步骤
- 预期行为与实际行为
- 环境信息（操作系统、Go 版本等）
- 相关日志或截图

### 提交代码

1. **Fork 本项目**
2. **创建分支**: `git checkout -b feature/your-feature`
3. **编写代码**: 确保遵循项目的编码规范
4. **添加测试**: 为新功能添加单元测试
5. **提交**: 编写清晰的 commit message
6. **推送**: `git push origin feature/your-feature`
7. **PR**: 创建 Pull Request

## 编码规范

### Go 代码规范

- 遵循 [Effective Go](https://golang.org/doc/effective_go) 指南
- 使用 `gofmt` 格式化代码
- 保持函数短小，职责单一
- 添加适当的注释（导出函数必须注释）
- 变量命名清晰，避免缩写

### 提交信息规范

```
<type>(<scope>): <subject>

<body>

<footer>
```

类型（type）：
- `feat`: 新功能
- `fix`: Bug 修复
- `docs`: 文档更新
- `style`: 代码格式调整
- `refactor`: 重构
- `test`: 测试相关
- `chore`: 构建工具或辅助工具的修改

示例：
```
feat(proxy): 添加正则表达式匹配支持

- 支持使用正则表达式进行 URL 匹配
- 兼容原有的字符串匹配方式

Closes #123
```

## 测试

运行测试：
```bash
go test ./...
```

运行测试并查看覆盖率：
```bash
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
```

## 代码检查

使用 golangci-lint 进行代码检查：
```bash
golangci-lint run ./...
```

## 开发环境

### 所需工具

- Go 1.25+
- golangci-lint（用于代码检查）
- make（使用 Makefile 构建）

### 构建项目

```bash
make build      # 标准构建
make build-opt  # 优化构建（体积更小）
make release    # 构建并安装到 ~/.local/bin
```

## 文档

- README.md: 项目主文档，包含安装、使用说明
- 代码注释: 使用 godoc 格式
- API 文档: 如有新增公共 API，需更新文档

## 行为准则

请遵守我们的 [行为准则](CODE_OF_CONDUCT.md)，保持社区友好和包容。

## 许可证

通过贡献代码，你同意你的代码使用 MIT 许可证。
