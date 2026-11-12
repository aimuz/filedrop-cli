# 互通测试向量

`vectors.json` 由服务器仓库的 `packages/crypto`（TypeScript 实现）生成，用来确保 Go 实现与之逐字节兼容：

```bash
# 在 filedrop 服务器仓库根目录
pnpm --filter @filedrop/api test      # 会写入 apps/api/test/vectors.json 与这里的副本
```

**这个文件不进版本库**：它包含约 300 KB 的随机密文，而且每次生成都不同，
提交它只会让仓库膨胀、且每次跑测试都产生无意义的 diff。

缺失时 `TestInteropDecryptVectors` 会失败并提示生成方式。想在 CI 里跑这个用例，
就在测试前执行上面那条命令。
