# 开发

## 环境

需要 Go **1.27** 或更新（代码用到了 `errors.AsType`、`new(expr)` 等新语法）。

## 常用命令

```bash
go test ./...          # 单元测试
go test -race ./...    # 并发相关改动建议带 -race
go vet ./...
go fix -diff ./...     # 应无输出：现代化改写都已合入
```

`go fix` 负责机械的现代化（`for i := 0; i < n; i++` → `for i := range n`、
`atomic.AddInt32(&x, 1)` → `x.Add(1)`、`errors.As` → `errors.AsType`）；
它看不到的（例如跨行声明的 `var err *T`）才需要手工改。

## 跨语言互通测试

互通测试读取 `internal/crypto/testdata/vectors.json`，由服务端的 `packages/crypto`
生成，**不提交到仓库**。生成方式：

```bash
# 在 filedrop 服务器仓库根目录
pnpm --filter @filedrop/api test
```

## 代码结构

| 目录 | 职责 |
| --- | --- |
| `cmd/filedrop` | 命令行入口：分发与信号处理（`main.go`）、命令表与帮助（`cli.go`）、各命令实现（`login.go` / `share.go` / `save.go` / `account.go`） |
| `internal/crypto` | 与 `packages/crypto` 逐字节兼容的加密协议 |
| `internal/share` | 加密并上传，产出链接 |
| `internal/filex` | 下载、解密、写盘（并行分块 + 断点重试） |
| `internal/api` | 服务端 HTTP 接口 |
| `internal/httpx` | 访问对象存储的 HTTP 客户端（分片并发用） |
| `internal/auth` | 设备码登录 |
| `internal/config` | 本地配置 |
| `internal/i18n` | 界面文案与语言检测（`zh` / `en`） |
| `internal/term` | 终端输出：颜色、进度、口令输入、剪贴板 |

## 文案与翻译

界面文案集中在 `internal/i18n/catalog.go`（字段定义）、`zh.go` 与 `en.go`（两种语言）。
新增或修改文案时：

1. 在对应的 `*Text` 结构里加字段；
2. 在 `zh.go` 和 `en.go` 里都填上；
3. 跑 `go test ./internal/i18n`——它会检查空字段，并确保英文文案里没有残留中文。

语言按 gettext 的约定只读环境变量：`FILEDROP_LANG` → `LANGUAGE` → `LC_ALL` →
`LC_MESSAGES` → `LANG`。`LANGUAGE` 在 C/POSIX locale 下被忽略；未支持的语言回退英文，
完全没有 locale 时回退中文。没有 `--lang` 开关。

## 基准测试

性能相关改动请附上前后对比。默认的 `go test` 不会跑基准测试：

```bash
go test ./internal/crypto -bench . -benchmem
go test ./internal/share  -bench Hash -benchmem
```
