# 大文件上传服务

一个基于 HTTP 的 Go 大文件上传示例，包含浏览器前端、分片上传、断点续传、相同文件秒传、服务端合并与 SHA-256 校验。

## 运行

```bash
go test ./...
go run . -addr :8080 -data data
```

浏览器打开 `http://localhost:8080`。

## 接口

### POST `/api/upload/init`

初始化上传。服务端会根据 `fileHash` 查询已有记录：

- 已完成且最终文件存在：返回 `instant=true`，前端无需再次上传。
- 未完成：返回已收到的分片编号，前端跳过这些分片继续上传。

请求：

```json
{
  "fileName": "demo.zip",
  "fileSize": 10485760,
  "fileHash": "sha256 hex",
  "chunkSize": 4194304,
  "totalChunks": 3
}
```

### POST `/api/upload/chunk`

上传单个分片，`multipart/form-data` 字段：

- `fileHash`
- `index`
- `chunk`

服务端将分片保存为 `data/chunks/{fileHash}/{index}.part`，写入临时文件后原子重命名。

### POST `/api/upload/complete`

所有分片上传完成后合并文件。服务端按编号顺序合并，计算 SHA-256，与 `fileHash` 不一致则拒绝完成。

### GET `/api/upload/status?hash=...`

查询上传状态和已上传分片。

## 设计说明

### 断点续传

每个文件使用内容 SHA-256 作为上传 ID。服务端持久化 `metadata.json`，记录文件名、大小、分片大小、总分片数、已上传分片集合和最终文件路径。浏览器刷新、网络中断或暂停后，重新调用初始化接口即可拿到 `uploadedChunks`，只上传缺失分片。

### 秒传

当 `fileHash` 对应的记录状态为 `completed`，并且最终文件仍存在时，初始化接口直接返回 `instant=true` 和下载地址。浏览器据此跳过分片上传。

### 大文件处理

浏览器按固定大小切片上传，服务端逐分片落盘，合并时使用流式 `io.Copy`，不会把完整文件读入内存。当前前端为便于演示使用 Web Crypto 一次性计算 SHA-256，生产环境可替换为 Web Worker 中的增量哈希，避免超大文件哈希阶段占用较多浏览器内存。

### 存储

项目使用本地文件系统保存分片和成品文件，使用 JSON 文件作为轻量元数据存储。题目不限制数据库类型，后续可将 `metadataStore` 替换为 SQLite、PostgreSQL 或 Redis，HTTP 协议和前端逻辑无需变化。

### 目录结构

```text
.
├── main.go
├── main_test.go
├── static/
│   └── index.html
├── README.md
└── data/
    ├── chunks/
    ├── files/
    └── metadata.json
```
