package i18n

var zhCatalog = Catalog{
	CLI: CLIText{
		Usage: `filedrop — 端到端加密的文件传递

用法：
  filedrop login                       登录账号（打开网页授权）
  filedrop whoami                      查看当前登录状态
  filedrop logout                      退出登录
  filedrop share <文件>...             上传并输出加密链接（默认 24 小时、不限次数）
  filedrop share --clipboard           把剪贴板里的文本当成文件分享
  filedrop save <链接 | ID[#密钥]>     下载并解密到当前目录
  filedrop list                        列出账号下的文件（只有 ID、大小与过期时间）
  filedrop rm <ID>...                  删除账号下的文件，立即释放容量
  filedrop quota                       查看账号容量
  filedrop version                     打印版本

命令选项：
  filedrop help <命令>                 查看某个命令的选项，例如 filedrop help save
`,
		UsageLine:      "用法：%s",
		OptionsHeader:  "选项：",
		NoSuchCommand:  "没有这个命令：%q",
		UnknownCommand: "未知命令 %q，运行 filedrop help 查看用法",
		SeeHelpHint:    "%v（运行 filedrop help 查看用法）",
		HelpOnlyOne:    "help 只接受一个命令名，多余参数：%v",
		SaveOnlyOne:    "save 只接受一个链接，多余参数：%v",
		Canceled:       "已取消。",

		SynLogin:   "filedrop login [--no-browser]",
		SynLogout:  "filedrop logout",
		SynWhoami:  "filedrop whoami",
		SynShare:   "filedrop share <文件>... | --clipboard [选项]",
		SynSave:    "filedrop save <链接 | ID[#密钥]> [--out DIR] [--password PW]",
		SynList:    "filedrop list",
		SynRm:      "filedrop rm <ID>...",
		SynQuota:   "filedrop quota",
		SynHelp:    "filedrop help <命令>",
		SynVersion: "filedrop version",

		SumLogin:   "登录账号。会打开浏览器，在那里面选 GitHub 或 Google，确认授权后命令行就登好了。",
		SumLogout:  "退出登录，并清掉本机保存的登录信息。已经发出去的文件不受影响。",
		SumWhoami:  "查看当前登录的是哪个账号。",
		SumShare:   "发送文件，输出可以直接发给别人的链接。每个文件各得一个链接，默认 24 小时后自动删除。",
		SumSave:    "收到链接后把文件下载下来。不会覆盖同名文件，重名时自动改成「名字 (1).扩展名」。",
		SumList:    "列出账号里还没过期的文件。只显示 ID、大小和剩余时间——文件名是加密的，只有你自己能看到。",
		SumRm:      "删除文件，立刻释放占用的空间。链接随即失效，已经下载走的文件不受影响。可以一次删多个。",
		SumQuota:   "查看已用空间和上限，以及账号里有多少个文件。",
		SumHelp:    "查看某个命令的详细用法，例如 filedrop help save。不带命令名时打印总览。",
		SumVersion: "打印当前版本号。",

		FlagPassword:     "用口令保护链接",
		FlagMaxDownloads: "限制下载次数（0 表示不限）",
		FlagCopy:         "上传成功后把链接复制到剪贴板",
		FlagClipboard:    "分享剪贴板里的文本",
		FlagOut:          `保存目录，默认当前目录；"-" 写到标准输出`,
		FlagSavePassword: "口令模式的文件用这个口令解密",
		FlagConcurrency:  "同时下载的分块数（0 表示自动，默认 4）",
		FlagNoBrowser:    "只打印链接，不自动打开浏览器",

		AuthComplete:    "✓ 认证完成。",
		LoggedInAs:      "✓ 已登录：%s",
		LoggedOut:       "已退出登录。",
		NotLoggedIn:     "当前没有登录。",
		CurrentAccount:  "已登录：%s",
		NeedLogin:       "尚未登录，先运行 filedrop login",
		ShareNeedsLogin: "分享需要登录，先运行 filedrop login（未登录也可以直接在网页上上传）",

		SetPasswordPrompt:    "设置口令（至少 8 位）：",
		RepeatPasswordPrompt: "再输入一次：",
		PasswordMismatch:     "两次输入的口令不一致",
		PasswordTooShort:     "口令至少 8 位",
		SavePasswordPrompt:   "口令：",
		PasswordHint:         "口令不要和链接放在同一条消息里。",
		KeyHint:              "链接本身就含密钥，直接发给对方即可。",

		CopyFailed:           "复制失败：%s",
		ClipboardReadFailed:  "读取剪贴板失败: %w",
		MaxDownloadsNegative: "--max-downloads 不能是负数",
		ConcurrencyNegative:  "--concurrency 不能是负数",
		Saved:                "已保存：%s",

		NoFiles:    "账号下没有未过期的文件。",
		ColID:      "ID",
		ColSize:    "大小",
		ColExpires: "过期时间",

		Deleted:      "%s 已删除",
		DeleteFailed: "%d 个文件没有删掉",
		QuotaLine:    "%s / %s，共 %d 个文件",
	},
	Share: ShareText{
		DirectoryNotFile: "不能分享目录，请指定文件",
		FileTooLarge:     "文件超过单文件上限 %s",
		ContentTooLarge:  "内容超过单文件上限 %s",
		QuotaExceeded:    "账号容量不足：已用 %s / %s。先运行 filedrop rm <ID> 删除旧文件，或等它们过期",
		QuotaHint:        "%s（运行 filedrop quota 看容量，或用 filedrop rm 删除旧文件）",
		ReadFailed:       "读取 %s 失败: %w",
		FileChanged:      "读取 %s 失败：文件在传输期间被改动",
		UploadPartFailed: "上传分片失败: %w",
		UploadPartStatus: "上传分片失败: HTTP %d %s",
		MissingETag:      "对象存储没有返回 ETag（检查桶的 CORS 是否暴露 ETag）",
		MissingPartURL:   "服务端没有返回分片地址",
		ProgressLabel:    "上传",
	},
	Filex: FilexText{
		MissingKey:          "这个链接缺少 # 之后的密钥，无法解密；请让发送方重新发一次完整链接",
		EmptyTarget:         "没有指定要下载的链接或 ID",
		BadURL:              "链接格式不对: %w",
		MissingFileID:       "链接里找不到文件 ID（应为 /d/<id>）",
		UnrecognizedTarget:  "无法识别这个链接或 ID",
		BadKey:              "链接里的密钥格式不对，请确认链接完整",
		NotFound:            "文件不存在、已过期或已被删除",
		PasswordRequired:    "这个文件需要口令",
		DownloadUnavailable: "文件已过期、已被删除，或已达到下载次数上限",
		SizeMismatch:        "解密后的大小与元数据不一致，文件可能不完整",
		ChunkDownloadFailed: "第 %d/%d 块下载失败: %w",
		ChunkVerifyFailed:   "第 %d/%d 块校验失败: %w",
		StdoutConcurrent:    "标准输出不支持并发写入",
		HTTPStatus:          "下载失败: HTTP %d",
		TransferFailed:      "下载失败: %w",
		RangeIgnored:        "对象存储忽略了 Range 请求，无法分块下载",
		ProgressLabel:       "下载",
	},
	Auth: AuthText{
		OpenLink:        "在浏览器里打开下面的链接完成登录：",
		DeviceCode:      "设备码 %s，%d 分钟内有效。",
		BrowserFallback: "无法自动打开浏览器，请手动复制上面的链接。",
		BrowserOpened:   "已尝试打开浏览器，等待授权…",
		NoToken:         "服务端没有返回登录令牌",
		Denied:          "登录请求被拒绝",
		Expired:         "登录请求已过期，请重新执行 filedrop login",
	},
	Config: ConfigText{
		NoConfigDir: "找不到配置目录: %w",
		Corrupt:     "配置文件损坏（%s）: %w",
	},
	Crypto: CryptoText{
		WrongPassword:    "口令不正确",
		BadFileID:        "文件 ID 格式不对",
		MasterKeyLength:  "主密钥必须是 32 字节",
		ChunkTooLarge:    "第 %d 块超过 8 MiB",
		ChunkNotFull:     "非最后一块第 %d 块必须是 8 MiB",
		ChunkTooShort:    "第 %d 块密文短于认证标签",
		ChunkAuthFailed:  "第 %d 块校验失败，文件可能被篡改或链接不完整",
		MetaTooLong:      "元数据过长",
		MetaBadBase64:    "元数据不是合法的 base64url",
		MetaTooShort:     "元数据密文过短",
		MetaAuthFailed:   "元数据解密失败：密钥不对或文件被篡改",
		MetaBadJSON:      "元数据不是合法的 JSON",
		MetaVersion:      "不支持的元数据版本",
		BadName:          "文件名不合法",
		BadSize:          "文件大小不合法",
		BadType:          "文件类型不合法",
		BadChunkSize:     "块大小不合法",
		BadChunkCount:    "块数不合法",
		PasswordTooShort: "口令至少 8 位",
		BadWrapSalt:      "口令外壳的盐不合法",
		BadWrapCipher:    "口令外壳的密文不合法",
		BadWrapLength:    "口令外壳密文长度不对",
		UnsupportedKDF:   "不支持的口令派生算法 %q",
		WeakKDF:          "口令派生迭代次数过低",
	},
	API: APIText{
		ConnectFailed: "连接 %s 失败: %w",
		DecodeFailed:  "解析响应失败: %w",
	},
	Term: TermText{
		EmptyInput:      "没有读到输入",
		Expired:         "已过期",
		LessThanMinute:  "不到 1 分钟",
		MinutesLeft:     "还有 %d 分钟",
		HoursLeft:       "还有 %d 小时",
		DaysLeft:        "还有 %d 天",
		NoClipboardTool: "找不到可用的剪贴板工具",
		ClipboardEmpty:  "剪贴板是空的（只支持文本内容）",
	},
}
