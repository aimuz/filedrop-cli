package crypto

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// 互通向量由服务器仓库的 packages/crypto（TypeScript）生成：
//
//	pnpm --filter @filedrop/api test   # apps/api/test/go-vectors.test.ts 会写入 testdata/vectors.json
//
// 这里验证 Go 实现能与网页端互相解密，任何字节级差异都会让测试失败。
type vectorFile struct {
	FileID    string `json:"fileId"`
	MasterKey string `json:"masterKey"`
	Password  string `json:"password"`
	Cases     []struct {
		Name       string   `json:"name"`
		Size       int64    `json:"size"`
		ChunkCount int      `json:"chunkCount"`
		Encrypted  int64    `json:"encryptedSize"`
		Chunks     []string `json:"chunks"`
		MetaCipher string   `json:"metaCiphertext"`
		Wrap       struct {
			KDF        string `json:"kdf"`
			Salt       string `json:"salt"`
			WrappedKey string `json:"wrappedKey"`
		} `json:"wrap"`
	} `json:"cases"`
}

func loadVectors(t *testing.T) vectorFile {
	t.Helper()
	path := filepath.Join("testdata", "vectors.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取测试向量失败（%v）。生成方式见 testdata/README.md：\n"+
			"  在服务器仓库根目录运行 pnpm --filter @filedrop/api test", err)
	}
	var vectors vectorFile
	if err := json.Unmarshal(raw, &vectors); err != nil {
		t.Fatalf("解析测试向量失败: %v", err)
	}
	return vectors
}

// prefixFor 与生成向量时的填充方式一致。
func prefixFor(size int64) []byte {
	pattern := []byte("filedrop-interop")
	out := make([]byte, size)
	for i := int64(0); i < size; i += int64(len(pattern)) {
		copy(out[i:], pattern[:min(int64(len(pattern)), size-i)])
	}
	return out
}

func TestInteropDecryptVectors(t *testing.T) {
	vectors := loadVectors(t)
	master, err := FromBase64URL(vectors.MasterKey)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range vectors.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			if got := ExpectedChunkCount(tc.Size); got != tc.ChunkCount {
				t.Fatalf("块数不一致: go=%d ts=%d", got, tc.ChunkCount)
			}
			if got := EncryptedSizeFor(tc.Size); got != tc.Encrypted {
				t.Fatalf("密文长度不一致: go=%d ts=%d", got, tc.Encrypted)
			}
			keys, err := DeriveKeys(master, vectors.FileID)
			if err != nil {
				t.Fatal(err)
			}
			meta, err := keys.OpenMeta(vectors.FileID, tc.MetaCipher)
			if err != nil {
				t.Fatalf("解密 TS 元数据失败: %v", err)
			}
			if meta.Size != tc.Size || meta.ChunkCount != tc.ChunkCount {
				t.Fatalf("元数据不一致: %+v", meta)
			}
			var plain []byte
			for index, sealedB64 := range tc.Chunks {
				sealed, err := FromBase64URL(sealedB64)
				if err != nil {
					t.Fatal(err)
				}
				last := index == len(tc.Chunks)-1
				chunk, err := keys.OpenChunk(vectors.FileID, index, last, sealed)
				if err != nil {
					t.Fatalf("第 %d 块解密失败: %v", index, err)
				}
				plain = append(plain, chunk...)
			}
			if int64(len(plain)) < tc.Size {
				t.Fatalf("解密后长度 %d 小于元数据 %d", len(plain), tc.Size)
			}
			plain = plain[:tc.Size]
			// 明文是重复的 "filedrop-interop" 前缀，便于校验。
			want := prefixFor(tc.Size)
			if !bytes.Equal(plain, want) {
				t.Fatalf("解密内容不对: %x", plain[:min(len(plain), 32)])
			}
		})
	}

	t.Run("password-wrap", func(t *testing.T) {
		wrap := vectors.Cases[0].Wrap
		key, err := UnwrapMasterKey(Wrap{KDF: wrap.KDF, Salt: wrap.Salt, WrappedKey: wrap.WrappedKey}, vectors.Password)
		if err != nil {
			t.Fatalf("解开 TS 的口令外壳失败: %v", err)
		}
		if !bytes.Equal(key, master) {
			t.Fatal("解出的主密钥与原始不一致")
		}
		if _, err := UnwrapMasterKey(Wrap{KDF: wrap.KDF, Salt: wrap.Salt, WrappedKey: wrap.WrappedKey}, "wrong-password"); err != ErrWrongPassword {
			t.Fatalf("错误口令应返回 ErrWrongPassword，得到 %v", err)
		}
	})
}

func TestRoundTrip(t *testing.T) {
	master, err := RandomBytes(MasterKeyLength)
	if err != nil {
		t.Fatal(err)
	}
	fileID := "fQdGQ7ALzoyhsnN01te_GA"
	// 故意用超过一块的长度，覆盖最后一块填充。
	plain := bytes.Repeat([]byte("filedrop"), (ChunkSize+12345)/8)
	chunks := ExpectedChunkCount(int64(len(plain)))
	keys, err := DeriveKeys(master, fileID)
	if err != nil {
		t.Fatal(err)
	}
	meta := NewMeta("note.txt", int64(len(plain)), "text/plain", chunks)
	sealedMeta, err := keys.SealMeta(fileID, meta)
	if err != nil {
		t.Fatal(err)
	}

	var out []byte
	lastPlain := PaddedLastChunkSize(int64(len(plain)))
	for index := 0; index < chunks; index++ {
		last := index == chunks-1
		start := index * ChunkSize
		end := min(start+ChunkSize, len(plain))
		content := plain[start:end]
		length := ChunkSize
		if last {
			length = lastPlain
		}
		buf := make([]byte, length)
		copy(buf, content)
		sealed, err := keys.SealChunk(fileID, index, last, buf)
		if err != nil {
			t.Fatal(err)
		}
		opened, err := keys.OpenChunk(fileID, index, last, sealed)
		if err != nil {
			t.Fatal(err)
		}
		if last {
			opened = opened[:len(content)]
		}
		out = append(out, opened...)
	}
	if !bytes.Equal(out, plain) {
		t.Fatal("往返后内容不一致")
	}

	opened, err := keys.OpenMeta(fileID, sealedMeta)
	if err != nil {
		t.Fatal(err)
	}
	if opened.Name != "note.txt" || opened.Size != int64(len(plain)) {
		t.Fatalf("元数据往返不一致: %+v", opened)
	}

	// 篡改一个字节必须失败。
	chunksSealed, err := keys.SealChunk(fileID, 0, chunks == 1, make([]byte, ChunkSize))
	if err != nil {
		t.Fatal(err)
	}
	chunksSealed[3] ^= 0xff
	if _, err := keys.OpenChunk(fileID, 0, chunks == 1, chunksSealed); err == nil {
		t.Fatal("被篡改的块必须解密失败")
	}
}

func TestPasswordWrapRoundTrip(t *testing.T) {
	master, err := RandomBytes(MasterKeyLength)
	if err != nil {
		t.Fatal(err)
	}
	wrap, err := WrapMasterKey(master, "correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	key, err := UnwrapMasterKey(*wrap, "correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(key, master) {
		t.Fatal("口令外壳往返不一致")
	}
	if wrap.KDF != "pbkdf2-sha256-600k" {
		t.Fatalf("kdf 字段不对: %s", wrap.KDF)
	}
	if _, err := WrapMasterKey(master, "short"); err == nil {
		t.Fatal("短口令必须被拒绝")
	}
}

func TestPaddedSizes(t *testing.T) {
	cases := []struct {
		size   int64
		padded int
		chunks int
	}{
		{0, PadBlock, 1},
		{1, PadBlock, 1},
		{PadBlock, PadBlock, 1},
		{PadBlock + 1, 2 * PadBlock, 1},
		{ChunkSize, ChunkSize, 1},
		{ChunkSize + 1, PadBlock, 2},
	}
	for _, tc := range cases {
		if got := PaddedLastChunkSize(tc.size); got != tc.padded {
			t.Errorf("size %d: padded=%d want %d", tc.size, got, tc.padded)
		}
		if got := ExpectedChunkCount(tc.size); got != tc.chunks {
			t.Errorf("size %d: chunks=%d want %d", tc.size, got, tc.chunks)
		}
	}
}
