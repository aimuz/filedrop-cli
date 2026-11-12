# 互通测试向量

`vectors.json` 由服务器仓库的 `packages/crypto`（TypeScript 实现）生成，用来确保 Go 实现与之逐字节兼容：

```bash
# 在 filedrop 服务器仓库根目录
pnpm --filter @filedrop/api test      # 会写入 apps/api/test/vectors.json 与这里的副本
```

文件里包含约 300 KB 的密文，因此不进版本库。缺失时 `TestInteropDecryptVectors` 会失败并提示生成方式。
