package share

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

// hashContent 的结果会交给服务端做黑名单与去重，算错会静默失效。
// 这里对着标准库的参考值校验各个边界（空文件、缓冲边界、多块）。
func TestHashContentMatchesReference(t *testing.T) {
	dir := t.TempDir()
	sizes := []int{0, 1, hashBufferSize - 1, hashBufferSize, hashBufferSize + 1, 3*hashBufferSize + 12345}
	for _, size := range sizes {
		path := filepath.Join(dir, "blob")
		data := make([]byte, size)
		for i := range data {
			data[i] = byte(i * 31)
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		content, err := FromPath(path, "https://filedrop.app")
		if err != nil {
			t.Fatal(err)
		}
		got, err := hashContent(content)
		if err != nil {
			t.Fatalf("size %d: %v", size, err)
		}
		want := sha256.Sum256(data)
		if got != hex.EncodeToString(want[:]) {
			t.Fatalf("size %d: got %s want %x", size, got, want)
		}
	}
}
