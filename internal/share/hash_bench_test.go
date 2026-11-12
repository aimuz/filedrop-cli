package share

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"testing"
)

// 上传前的整文件 SHA-256 是对外承诺的耗时：用户敲完回车要等它算完才开始传。
func BenchmarkHashContent8MiB(b *testing.B) {
	dir := b.TempDir()
	path := dir + "/blob"
	file, err := os.Create(path)
	if err != nil {
		b.Fatal(err)
	}
	chunk := make([]byte, 1<<20)
	for i := range chunk {
		chunk[i] = byte(i)
	}
	for range 8 {
		file.Write(chunk)
	}
	file.Close()

	content, err := FromPath(path, "https://filedrop.app")
	if err != nil {
		b.Fatal(err)
	}

	b.SetBytes(content.size)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := hashContent(content); err != nil {
			b.Fatal(err)
		}
	}
}

// 同样 8 MiB，但直接从内存读：区分「哈希本身」与「读文件的 syscall 开销」。
func BenchmarkSHA256Memory8MiB(b *testing.B) {
	data := make([]byte, 8<<20)
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		digest := sha256.New()
		digest.Write(data)
		_ = hex.EncodeToString(digest.Sum(nil))
	}
}

func BenchmarkSHA256DirectMemory8MiB(b *testing.B) {
	data := make([]byte, 8<<20)
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = sha256.Sum256(data)
	}
}

func BenchmarkIOCopyOnly8MiB(b *testing.B) {
	data := make([]byte, 8<<20)
	reader := &memReader{data: data}
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		reader.off = 0
		io.Copy(io.Discard, reader)
	}
}

type memReader struct {
	data []byte
	off  int
}

func (r *memReader) Read(p []byte) (int, error) {
	if r.off >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.off:])
	r.off += n
	return n, nil
}
