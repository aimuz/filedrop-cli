package crypto

import (
	"testing"
)

func BenchmarkOpenChunkAllocPerChunk(b *testing.B) {
	master := make([]byte, 32)
	fileID := "fQdGQ7ALzoyhsnN01te_GA"
	keys, _ := DeriveKeys(master, fileID)
	plain := make([]byte, ChunkSize)
	sealed, err := keys.SealChunk(fileID, 0, true, plain)
	if err != nil {
		b.Fatal(err)
	}
	b.SetBytes(int64(len(plain)))
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		// 模拟「每块都传 nil」：分配一块新的明文缓冲。
		out, err := keys.OpenChunkTo(nil, fileID, 0, true, sealed)
		if err != nil {
			b.Fatal(err)
		}
		_ = out
	}
}

func BenchmarkOpenChunkReusedBuffer(b *testing.B) {
	master := make([]byte, 32)
	fileID := "fQdGQ7ALzoyhsnN01te_GA"
	keys, _ := DeriveKeys(master, fileID)
	plain := make([]byte, ChunkSize)
	sealed, err := keys.SealChunk(fileID, 0, true, plain)
	if err != nil {
		b.Fatal(err)
	}
	var out []byte
	b.SetBytes(int64(len(plain)))
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		out, err = keys.OpenChunkTo(out, fileID, 0, true, sealed)
		if err != nil {
			b.Fatal(err)
		}
	}
	_ = out
}
