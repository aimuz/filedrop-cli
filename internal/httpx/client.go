// Package httpx 提供访问对象存储的分片传输客户端。
package httpx

import "net/http"

// Client 供上传、下载分片共用。
//
// http.DefaultTransport 每个 host 只保留 2 条空闲连接（MaxIdleConnsPerHost 默认 2），
// 而并行分块同时会开 4 条以上；多出来的连接用完就被关掉，下一批又要重新握手
// （TLS 握手尤其贵）。这里只把空闲连接数调大，其余保持默认行为。
var Client = func() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 32
	transport.MaxIdleConnsPerHost = 8
	return &http.Client{Transport: transport}
}()
