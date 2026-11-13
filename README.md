# filedrop CLI

[filedrop](https://filedrop.app) 的命令行客户端：在终端里把文件发给别人，对方点开链接就能下载。

- **只有拿到链接的人能打开**：文件在你自己的电脑上加密后才发出去，服务器和中间的任何人都看不到内容。
- **和网页互通**：命令行生成的链接可以直接用浏览器打开；网页上生成的文件，也能用命令行下载下来。
- **够用就停**：默认 24 小时后自动删除，你可以随时提前删掉。

## 安装

需要先安装 [Go](https://go.dev/dl/)（1.27 或更新）：

```bash
go install github.com/aimuz/filedrop-cli/cmd/filedrop@latest
```

然后在终端里运行 `filedrop`，能看到用法说明就说明装好了。

## 用法

```bash
filedrop login                  # 登录（会在浏览器里打开一个授权页面）
filedrop share 报告.pdf         # 发送文件，得到一个链接
filedrop save <链接>            # 收到链接后，把文件下载下来
```

### 发送文件

```bash
filedrop share 报告.pdf
```

会输出一行链接，把它发给对方就行：

```
https://filedrop.app/d/aBcDeFgHiJkLmNoPqRsTuV#xYz...
```

一次可以发多个文件，每个文件各得一个链接：

```bash
filedrop share 报告.pdf 照片.zip
```

**想限制谁能打开？**

| 做法 | 适合 | 用法 |
| --- | --- | --- |
| 设置口令 | 想多一层保护 | `filedrop share 报告.pdf --password` |
| 限制次数 | 只给一个人下载 | `filedrop share 报告.pdf --max-downloads 1` |

设置口令后，链接里就不带密钥了，对方需要输入你另外告诉他的口令才能打开。
**口令不要和链接放在同一条消息里发**，否则就失去了保护的意义。

**发送剪贴板里的内容**（比如刚复制的一段文字、一段代码）：

```bash
filedrop share --clipboard
```

**把链接直接复制到剪贴板**，省得手动选中：

```bash
filedrop share 报告.pdf --copy
```

大文件会分片并行上传，不用等前一块传完再传下一块。传输中按 `Ctrl-C` 可以随时取消。
一次分享多个文件时，`--copy` 会把所有链接一起放进剪贴板（每行一个）。

### 下载文件

```bash
filedrop save "https://filedrop.app/d/aBcDeFgHiJkLmNoPqRsTuV#xYz..."
```

- **链接要加引号**：链接里有 `#`，不加引号的话后面的部分会被终端当成注释丢掉。
- 文件会保存到当前目录。**不会覆盖已有文件**，重名时自动变成 `报告 (1).pdf`。
- 想存到别的目录：`filedrop save <链接> --out ~/Downloads`
- 想直接交给别的程序处理（比如解压、转格式）：`filedrop save <链接> --out -` 会把内容输出到终端，可以接管道。
- 如果对方发来的是**口令链接**（链接里没有 `#` 那一段），运行后按提示输入口令即可。

网速不好也没关系：下载中断只会重试那一小段，不用从头再来。想快一点可以调高 `--concurrency`（默认为 4），比如 `--concurrency 8`。传输中按 `Ctrl-C` 会取消下载并删掉没写完的文件。

### 管理已经发出的文件

```bash
filedrop list      # 看看账号里还有哪些文件
filedrop rm <ID>   # 删除某个文件，立刻腾出空间（可以一次删多个）
filedrop quota     # 已用多少空间
```

`filedrop list` 里只显示文件的 ID、大小和过期时间——**文件名只有你自己能看到**，因为它在加密之后才发出去。

### 其他

```bash
filedrop whoami        # 当前登录的是哪个账号
filedrop logout        # 退出登录
filedrop help save     # 查看某个命令的所有选项
filedrop version       # 版本号
```

### 界面语言

和大多数 Unix 工具一样，命令行按 gettext 的环境变量决定界面语言，没有 `--lang` 开关：

```bash
LANG=zh_CN.UTF-8 filedrop share 报告.pdf   # 跟随系统 locale
LC_ALL=C filedrop share 报告.pdf           # 脚本里强制英文
FILEDROP_LANG=en filedrop help save        # 只覆盖 filedrop
```

查找顺序是 `FILEDROP_LANG` → `LANGUAGE` → `LC_ALL` → `LC_MESSAGES` → `LANG`。
`LANGUAGE` 是冒号分隔的优先级列表，locale 为 C/POSIX 时不做翻译、直接英文。
目前支持简体中文（`zh`）和英文（`en`），其它语言回退英文。它只影响终端提示与错误信息，
不影响加密格式和生成的链接。

## 常见问题

**链接被谁看到，谁就能下载吗？**
是的。链接里包含了打开文件所需的钥匙，所以把它当作密码来对待。想让链接本身失去作用，删掉文件即可。

**文件什么时候消失？**
默认 24 小时后自动删除。你也可以用 `filedrop rm` 提前删掉。

**空间不够了怎么办？**
账号一共有 50 GiB 空间。删掉不用的文件（`filedrop rm`）会立刻释放空间，或者等它们到期自动释放。

**我能在网页上打开命令行发来的链接吗？**
可以，直接粘到浏览器地址栏就行。反过来，网页上发的文件也能用 `filedrop save` 下载。

**用 GitHub 还是 Google 登录？**
都可以，登录时在浏览器里选。登录一次之后，命令行会记住你。

## 需要更多细节？

- 想了解加密方式和接口细节，见 [`docs/PROTOCOL.md`](docs/PROTOCOL.md)。
- 想参与开发，见[开发说明](CONTRIBUTING.md)。

## 许可证

[Apache-2.0](LICENSE)
