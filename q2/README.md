# q2 自定义 Agent Loop

这是一个用 Go 实现的最小可用 agent loop。它提供 CLI，可以通过命令行与模型交互；内置 tools 模块，支持文件读写、目录查看和命令执行；提供 providers 模块，可以切换 mock provider、OpenAI-compatible provider 和阿里云百炼 Qwen provider；还预留了 skills 扩展。

## 运行

```bash
go test ./...
go run . -tools
go run . -run "list ."
go run . -run "read README.md"
go run . -run "shell go test ./..."
```

交互模式：

```bash
go run .
```

进入后可输入：

```text
list .
read README.md
write tmp/hello.txt hello
shell go version
/tools
/exit
```

默认使用 `mock` provider，便于离线演示 agent loop 和工具调用。

## 使用真实模型

### 阿里云百炼 Qwen Plus

项目已内置阿里云百炼兼容 OpenAI 的默认配置：

- provider：`qwen`，也可用 `dashscope`、`bailian`、`aliyun`
- 默认模型：`qwen-plus`
- 默认地址：`https://dashscope.aliyuncs.com/compatible-mode/v1`
- API Key 环境变量：`DASHSCOPE_API_KEY`

PowerShell：

```powershell
$env:DASHSCOPE_API_KEY="your_dashscope_api_key"
go run . -provider qwen
```

也可以用环境变量指定：

```powershell
$env:DASHSCOPE_API_KEY="your_dashscope_api_key"
$env:AGENT_PROVIDER="qwen"
$env:AGENT_MODEL="qwen-plus"
go run .
```

如需使用其他地域或代理地址：

```powershell
$env:DASHSCOPE_BASE_URL="https://dashscope.aliyuncs.com/compatible-mode/v1"
```

### OpenAI-compatible

项目也实现了通用 OpenAI-compatible Chat Completions provider。配置环境变量后运行：

```bash
set OPENAI_API_KEY=your_api_key
set AGENT_PROVIDER=openai
set AGENT_MODEL=gpt-4o-mini
go run .
```

也可以通过参数指定：

```bash
go run . -provider openai -model gpt-4o-mini
```

如需使用兼容 OpenAI API 的其他服务商，可设置：

```bash
set OPENAI_BASE_URL=https://example.com/v1
```

## 架构设计

### Agent Loop

核心循环位于 `internal/agent`：

1. CLI 把用户输入追加到消息历史。
2. Runner 调用 provider 获取模型响应。
3. 如果响应包含 `tool_calls`，Runner 调用工具注册表执行工具。
4. 工具结果以 `tool` 消息追加回上下文。
5. 重复上述过程，直到模型返回最终答案或达到 `max-steps`。

模型调用工具时使用统一 JSON 格式：

```json
{
  "tool_calls": [
    {
      "name": "read_file",
      "arguments": {
        "path": "README.md"
      }
    }
  ]
}
```

### Tools 模块

工具注册表位于 `internal/tools`。每个工具包含：

- `Spec`：名称、描述、参数说明。
- `Handler`：实际执行逻辑。
- `Result`：结构化输出，包含成功状态、输出、错误和是否修改文件。

内置工具：

- `read_file`：读取 workspace 内文本文件。
- `write_file`：写入 workspace 内文本文件。
- `list_dir`：列出 workspace 内目录。
- `shell`：在 workspace 内执行命令并返回输出。

文件工具会拒绝绝对路径和 `..` 越界路径，避免 agent 修改 workspace 外部文件。

### Providers 模块

`internal/providers` 定义统一接口：

```go
type Provider interface {
    Chat(ctx context.Context, req Request) (Response, error)
}
```

当前实现：

- `mock`：无需网络，按 `read/list/write/shell` 前缀生成工具调用，适合测试和演示。
- `openai`：调用 OpenAI-compatible `/chat/completions` 接口。
- `qwen`：阿里云百炼 Qwen Plus 的快捷 provider，底层同样使用 OpenAI-compatible 接口。

后续接入 Claude、Gemini、本地模型时，只需要新增 Provider 实现，不需要修改 agent loop。

### Skills 模块

`internal/skills` 会加载 workspace 下 `skills/*.md`，提取文件名和首个非空行作为技能摘要，注入 system prompt。这样可以通过 Markdown 文件扩展 agent 的领域行为。

### MCP 扩展方向

当前版本没有直接接入 MCP 网络协议，但 tools 注册表已经是统一扩展点。后续可以新增 `internal/mcp`：

1. 读取 MCP server 配置。
2. 拉取远端 tool schema。
3. 将 MCP tool 转换为本项目的 `tools.Spec`。
4. Handler 中转发调用到 MCP server。

这样 agent loop 不需要感知工具来自本地还是 MCP。

## 目录结构

```text
q2/
├── go.mod
├── main.go
├── README.md
└── internal/
    ├── agent/
    ├── providers/
    ├── skills/
    └── tools/
```

## 安全说明

`shell` 工具会执行命令，适合本地可信环境演示。真实产品中建议增加命令白名单、人工确认、审计日志和沙箱隔离。
