# filedrop CLI

[filedrop](https://filedrop.app) 的命令行客户端：在终端里加密上传、拿到链接、需要时再下载解密。

- **端到端加密**：文件在本地按 8 MiB 分块，用 AES-256-GCM 加密后才上传；密钥只存在于下载链接的 `#` 之后，
  服务器、对象存储和网络中间人都看不到。
- **与网页互通**：生成的链接可以直接在浏览器打开；网页生成的链接也能用 `filedrop save` 下载。
- **账号容量**：登录后每次上传默认保存 24 小时、不限下载次数；账号总容量 50 GiB，文件到期或删除后释放。

## 安装

```bash
go install github.com/aimuz/filedrop-cli/cmd/filedrop@latest
```

或从源码：

```bash
git clone https://github.com/aimuz/filedrop-cli
cd filedrop-cli
go build -o filedrop ./cmd/filedrop
```

## 用法

```bash
filedrop login                     # 打开网页登录页，在那里面选 GitHub 或 Google
filedrop share ./report.pdf        # 上传，输出加密链接
filedrop share a.pdf b.zip         # 多个文件 → 多个链接，各自独立过期
filedrop save https://filedrop.app/d/<id>#<key>
filedrop save <id>#<key>           # 省略域名也可以
filedrop save <id>                 # 口令模式的链接：按提示输入口令
filedrop list                      # 账号下的文件：只有 ID 与大小
filedrop rm <id>...                # 删除并立即释放容量
filedrop quota                     # 已用 / 上限
```

### `filedrop share`

```bash
filedrop share <file>... [--password] [--max-downloads N] [--copy]
```

- 默认保存 **24 小时**、**不限**下载次数，输出 `https://filedrop.app/d/<id>#<key>`。
- `--password`：设置口令（至少 8 位，输入两遍），链接里不再带密钥；对方打开网页或
  `filedrop save <链接>` 时输入口令即可。口令不要和链接放在同一条消息里。
- `--max-downloads N`：限制下载次数。
- `--copy`：上传成功后把第一份链接复制到剪贴板（需要 `pbcopy` / `wl-copy` / `xclip`）。
- 选项放在文件名前后都可以。

链接本身就是密钥，谁拿到谁就能下载。想限制传播，用 `--password` 或 `--max-downloads`。

### `filedrop save`

```bash
filedrop save <链接 | ID[#密钥]> [-out DIR] [--password PW]
```

- 目标放在最前面，选项放后面。链接里的 `#` 在 shell 里是注释符，记得加引号；
  不加引号时后面的选项会被一起截掉。
- 默认保存到当前目录，**不覆盖同名文件**（自动改成 `a (1).pdf`）。
- `--out DIR` 指定目录，`--out -` 写到标准输出（适合管道）。
- 任意一块校验失败都会中止并删除已写入的临时文件。

### 其他命令

```bash
filedrop whoami     # 当前是否已登录
filedrop logout     # 撤销令牌并清除本地配置
filedrop version
```

## 数据与隐私

- `filedrop list` 只能看到文件 ID、大小与过期时间：文件名、类型都在加密元数据里，服务端读不到。
  只有拿到链接里的密钥（或口令）才能还原。
- 本地配置文件存放登录令牌，权限 `0600`：
  macOS `~/Library/Application Support/filedrop/config.json`，其他系统 `$XDG_CONFIG_HOME/filedrop/config.json`。
  可以用 `FILEDROP_CONFIG` 改路径。
- 密钥只出现在 `share` 的输出里，不会写入日志，也不会缓存到本地。

## 环境变量

| 变量 | 说明 |
| --- | --- |
| `FILEDROP_API_URL` | API 地址，默认 `https://filedrop.app` |
| `FILEDROP_CONFIG` | 配置文件路径 |
| `NO_COLOR` | 关闭彩色输出 |

## 登录是怎么工作的

1. `filedrop login` 请求服务端开一个设备码，得到授权链接并尝试打开浏览器；
2. 你在网页里选择 GitHub 或 Google 登录，确认「授权命令行登录」；
3. 命令行每 2 秒轮询一次，确认后拿一次性令牌写进本地配置（服务端只保存它的哈希）；
4. 设备码 5 分钟内有效，被拒绝或过期都会提示重新登录。

命令行全程不知道、也不关心你从哪个平台登录。

## 协议

加密细节见 [`docs/PROTOCOL.md`](docs/PROTOCOL.md)。上传/下载都走预签名 URL 直连对象存储，
服务端只处理小于 64 KB 的控制请求。

## 开发

```bash
go test ./...
go vet ./...
```

跨语言互通测试读取 `internal/crypto/testdata/vectors.json`（由服务端的
`packages/crypto` 生成，不提交到仓库）。生成方式：

```bash
# 在 filedrop 服务器仓库根目录
pnpm --filter @filedrop/api test
```

## 许可证

[Apache-2.0](LICENSE)
