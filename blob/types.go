package blob

import (
	"path"
	"strings"
)

// types is the one table from a file's name to a blob's type. The
// operating system's own table differs from machine to machine.
var types = map[string]string{
	".txt": "text/plain", ".log": "text/plain", ".md": "text/markdown", ".html": "text/html", ".htm": "text/html",
	".css": "text/css", ".csv": "text/csv", ".tsv": "text/tab-separated-values", ".xml": "application/xml",
	".json": "application/json", ".jsonl": "application/jsonl", ".ndjson": "application/jsonl",
	".yaml": "application/yaml", ".yml": "application/yaml", ".js": "text/javascript", ".mjs": "text/javascript",
	".pdf": "application/pdf", ".zip": "application/zip", ".tar": "application/x-tar", ".gz": "application/gzip",
	".tgz": "application/gzip", ".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".gif": "image/gif",
	".webp": "image/webp", ".svg": "image/svg+xml", ".bmp": "image/bmp", ".tiff": "image/tiff", ".ico": "image/x-icon",
	".mp3": "audio/mpeg", ".wav": "audio/wav", ".ogg": "audio/ogg", ".flac": "audio/flac", ".m4a": "audio/mp4",
	".mp4": "video/mp4", ".webm": "video/webm", ".mov": "video/quicktime", ".eml": "message/rfc822",
	".doc": "application/msword", ".xls": "application/vnd.ms-excel", ".ppt": "application/vnd.ms-powerpoint",
	".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	".pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
	".wasm": "application/wasm", ".sh": "text/x-shellscript", ".py": "text/x-python", ".go": "text/x-go",
}

// Bytes is the type of a blob nothing more is known about.
const Bytes = "application/octet-stream"

// TypeOf is the type of a blob made from a file with the name.
func TypeOf(name string) string {
	if t, ok := types[strings.ToLower(path.Ext(name))]; ok {
		return t
	}
	return Bytes
}
