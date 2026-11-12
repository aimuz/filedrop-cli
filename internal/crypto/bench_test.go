package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/hex"
	"runtime"
	"testing"
)

// 只是为了看清楚瓶颈在哪：AES-GCM 吞吐、SHA-256 吞吐、内存分配。
func BenchmarkAESGCMSeal8MiB(b *testing.B) {
	key := make([]byte, 32)
	block, _ := aes.NewCipher(key)
	aead, _ := cipher.NewGCM(block)
	nonce := make([]byte, aead.NonceSize())
	plain := make([]byte, ChunkSize)
	aad := make([]byte, 40)
	var out []byte

	b.SetBytes(int64(len(plain)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out = aead.Seal(out[:0], nonce, plain, aad)
	}
	_ = out
}

func BenchmarkSHA256Hash(b *testing.B) {
	data := make([]byte, ChunkSize)
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sum := sha256.Sum256(data)
		_ = hex.EncodeToString(sum[:])
	}
}

func BenchmarkSealChunkWithDerivedKeys(b *testing.B) {
	master := make([]byte, 32)
	fileID := "fQdGQ7ALzoyhsnN01te_GA"
	keys, _ := DeriveKeys(master, fileID)
	plain := make([]byte, ChunkSize)
	b.SetBytes(int64(len(plain)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := keys.SealChunk(fileID, 0, true, plain); err != nil {
			b.Fatal(err)
		}
	}
}

// SealChunkTo 是上传路径真正用的形式：复用密文缓冲区。
func BenchmarkSealChunkReusedBuffer(b *testing.B) {
	master := make([]byte, 32)
	fileID := "fQdGQ7ALzoyhsnN01te_GA"
	keys, _ := DeriveKeys(master, fileID)
	plain := make([]byte, ChunkSize)
	sealed := make([]byte, 0, CiphertextChunkSize)
	b.SetBytes(int64(len(plain)))
	b.ReportAllocs()
	b.ResetTimer()
	var err error
	for i := 0; i < b.N; i++ {
		sealed, err = keys.SealChunkTo(sealed, fileID, i, true, plain)
		if err != nil {
			b.Fatal(err)
		}
	}
	_ = sealed
}

func TestReportCPU(t *testing.T) {
	t.Logf("GOARCH=%s CPUs=%d", runtime.GOARCH, runtime.NumCPU())
}
