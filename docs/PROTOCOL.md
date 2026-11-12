# filedrop 加密协议

本文件描述命令行客户端使用的那部分协议，供其他语言的实现者参考。
权威实现有两份，两者逐字节兼容：

- TypeScript：服务端仓库里的 `packages/crypto`（网页端在用）；
- Go：本仓库的 `internal/crypto`。

## 1. 链接

```
https://<host>/<lang>/d/<fileId>#<masterKey>
```

- `fileId`：16 字节随机数，base64url 编码，22 个字符；
- `masterKey`：32 字节随机数，base64url 编码，43 个字符；
- 密钥只在 `#` 之后：浏览器不会把片段发给服务器，也不会出现在 Referer 里。

口令模式下链接**不带** `#<masterKey>`，主密钥由口令在客户端解开（见第 5 节）。

## 2. 派生密钥

```
salt     = fileId 的 16 字节
fileKey  = HKDF-SHA256(masterKey, salt, info = "filedrop/v1/file", 32 字节)
metaKey  = HKDF-SHA256(masterKey, salt, info = "filedrop/v1/meta", 32 字节)
```

两把密钥都用作 AES-256-GCM。

## 3. 分块加密

- 明文按 **8 MiB** 切块；空文件也有一块。
- 最后一块在加密前用**随机字节**补到 64 KiB 的整数倍（空文件补满 64 KiB），
  这样对象存储只能看到按 64 KiB 分桶的密文长度，拿不到精确明文大小。
- 其余每块的密文长度 = 8 MiB + 16 字节认证标签。

```
nonce = 4 字节 0x00 ‖ u32be(块序号 >> 32) ‖ u32be(块序号 & 0xffffffff)      // 12 字节
AAD   = "filedrop/v1" ‖ fileId(16) ‖ u32be(块序号) ‖ (最后一块 ? 0x01 : 0x00)
密文   = AES-256-GCM-Seal(fileKey, nonce, 明文块, AAD)
```

任一块认证失败都必须中止，并丢弃已写内容（可能是篡改、乱序、截断或拼接到别的文件）。

## 4. 元数据

明文元数据是 JSON，固定补空格到 **4096 字节**后再加密：

```json
{ "v": 1, "name": "report.pdf", "size": 123456, "type": "application/pdf",
  "chunkSize": 8388608, "chunkCount": 1 }
```

```
nonce    = 12 字节随机数
AAD      = "filedrop/v1" ‖ fileId(16)
密文     = AES-256-GCM-Seal(metaKey, nonce, 补满 4 KiB 的 JSON, AAD)
存放格式 = base64url(nonce ‖ 密文)
```

`metaCiphertext` 在「完成上传」时提交给服务端；服务端不知道文件名与真实大小。

## 5. 口令模式

口令模式只在客户端多一层「用口令包裹主密钥」，文件内容加密完全不变。

```
salt        = 16 字节随机数
kek         = PBKDF2-HMAC-SHA256(NFKC(password), salt, iterations = 600000, 32 字节)
nonce       = 12 字节随机数
AAD         = "filedrop/v1/key"          // 不含 fileId
wrappedKey  = base64url(nonce ‖ AES-256-GCM-Seal(kek, nonce, masterKey, AAD))

wrap = { "kdf": "pbkdf2-sha256-600k", "salt": base64url(salt), "wrappedKey": wrappedKey }
```

`wrap` 在创建文件时提交，服务端原样保存，并在 `GET /api/files/:id` 里返回；
没有链接密钥的客户端可以用口令解出 `masterKey` 再下载。
口令先做 Unicode **NFKC** 规范化，避免输入法差异导致解不开。

## 6. 上传流程

1. `POST /api/files`，请求体：

   ```json
   { "encryptedSize": 8388624, "chunkCount": 1,
     "sha256": "<明文 SHA-256，小写 hex>", "ttlSeconds": 86400,
     "maxDownloads": null, "wrap": null }
   ```

   `encryptedSize` 必须与 `chunkCount` 自洽：

   ```
   (chunkCount - 1) * (8388608 + 16) + 补块长度 + 16
   ```

   响应给出 `fileId`、`deleteToken`、`uploadId` 和若干预签名分片地址。

   登录用户（`Authorization: Bearer <token>`）走账号容量，保存时长固定 24 小时；
   匿名上传需要 Turnstile 令牌，按时长 1 小时 / 24 小时计。

2. 对每个分片发 `PUT <预签名地址>`，请求体是该块的密文（含 16 字节标签），
   从响应的 `ETag` 头里取出该分片的 etag（去掉引号）。地址用完后用
   `POST /api/files/:id/parts` 补签。

3. `POST /api/files/:id/complete`：

   ```json
   { "parts": [{ "partNumber": 1, "etag": "..." }],
     "metaCiphertext": "<第 4 节的 base64url>" }
   ```

## 7. 下载流程

1. `GET /api/files/:id` 拿到 `encryptedSize`、`chunkCount`、`metaCiphertext`、`wrap`；
2. 有链接密钥就直接派生；没有则先问口令，用 `wrap` 解出主密钥；
3. 用元数据密钥解开 `metaCiphertext`，得到文件名与真实大小；
4. `POST /api/files/:id/download` 拿到一次性预签名地址；
5. 按块 `Range: bytes=start-end` 下载，逐块认证解密，最后一块按真实大小裁掉填充。

## 8. 账号接口

命令行登录只用这些接口（全部不区分登录平台）：

| 接口 | 说明 |
| --- | --- |
| `POST /api/auth/device` | 申请设备码：`{ clientName }` → `{ code, authUrl, interval, expiresAt }` |
| `GET /api/auth/device?code=` | 轮询：`pending` / `authorized`（附一次性 `token`）/ `denied` / `expired` |
| `GET /api/auth/session` | 当前账号：`{ userId, displayName }` |
| `POST /api/auth/logout` | 撤销令牌 |
| `GET /api/account` | `{ usedBytes, quotaBytes, fileCount }` |
| `GET /api/account/files` | `{ files: [{ fileId, encryptedSize, expiresAt }] }`，只有这三个字段 |
| `DELETE /api/account/files/:id` | 删除并立即释放容量 |

令牌只在 `authorized` 那一次响应里出现，服务端只保存它的 SHA-256。
