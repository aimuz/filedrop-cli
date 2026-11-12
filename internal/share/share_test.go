package share

import "testing"

func TestParseUsage(t *testing.T) {
	used, quota, ok := parseUsage("account storage full: 786480 of 1048576 bytes used")
	if !ok || used != 786480 || quota != 1048576 {
		t.Fatalf("解析结果不对: %d %d %v", used, quota, ok)
	}
	if _, _, ok := parseUsage("daily upload volume reached for this network"); ok {
		t.Fatal("其它错误不应被当成配额信息")
	}
}
