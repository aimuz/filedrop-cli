// 用 TypeScript 实现（packages/crypto）生成互通测试向量。
//
// 用法：在仓库根目录运行
//   pnpm --filter @filedrop/crypto exec tsx apps/cli/internal/crypto/testdata/gen-vectors.mjs
// 或者用 vitest：见 apps/api/test/vectors.test.ts。
// 生成的 vectors.json 提交在 apps/cli 仓库里，Go 测试直接读取。
import { randomBytes } from 'node:crypto';
import { writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import {
  deriveFileKeys,
  encryptFileBytes,
  generateMasterKey,
  sealChunk,
  sealMeta,
  toBase64Url,
  wrapMasterKey,
  createFileMeta,
  expectedChunkCount,
  encryptedSizeFor,
} from '../../../../packages/crypto/src/index.ts';

const here = dirname(fileURLToPath(import.meta.url));
const fileId = 'fQdGQ7ALzoyhsnN01te_GA';
const masterKey = generateMasterKey();
const password = 'interop-password-2024';

function fill(size) {
  const out = Buffer.alloc(size);
  const pattern = Buffer.from('filedrop-interop');
  for (let i = 0; i < size; i += pattern.length) pattern.copy(out, i);
  return out;
}

const sizes = [0, 1, 65536, 8 * 1024 * 1024 + 12345];
const cases = [];
for (const size of sizes) {
  const plaintext = fill(size);
  const { chunks, metaCiphertext, meta } = await encryptFileBytes(plaintext, {
    fileId,
    masterKey,
    name: `interop-${size}.bin`,
    type: 'application/octet-stream',
  });
  cases.push({
    name: `size-${size}`,
    size,
    chunkCount: meta.chunkCount,
    encryptedSize: encryptedSizeFor(size),
    chunks: chunks.map((chunk) => toBase64Url(chunk)),
    metaCiphertext,
  });
}

// 口令外壳挂在第一个用例上，Go 侧用它验证 PBKDF2 互通。
const wrap = await wrapMasterKey(masterKey, password);
cases[0].wrap = wrap;

// 额外覆盖：单块加密的确定性检查所需（AAD/nonce 正确性）。
const { fileKey } = await deriveFileKeys(masterKey, fileId);
const sealed = await sealChunk(fileKey, fileId, 3, false, new Uint8Array(8 * 1024 * 1024));

const payload = {
  fileId,
  masterKey: toBase64Url(masterKey),
  password,
  chunkSize: 8 * 1024 * 1024,
  expectedChunkCountForHuge: expectedChunkCount(1024 * 1024 * 1024),
  openAtIdx3: toBase64Url(sealed),
  cases,
};

writeFileSync(join(here, 'vectors.json'), `${JSON.stringify(payload)}\n`);
console.log(`vectors.json 已生成：${cases.length} 个用例`);
