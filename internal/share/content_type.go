package share

import (
	"path/filepath"
	"strings"
)

// contentType 写进加密元数据，供浏览器下载时提示。
func contentType(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".pdf":
		return "application/pdf"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".svg":
		return "image/svg+xml"
	case ".txt", ".md", ".log":
		return "text/plain"
	case ".json":
		return "application/json"
	case ".html", ".htm":
		return "text/html"
	case ".zip":
		return "application/zip"
	case ".mp4":
		return "video/mp4"
	case ".mov":
		return "video/quicktime"
	case ".mp3":
		return "audio/mpeg"
	default:
		return "application/octet-stream"
	}
}
