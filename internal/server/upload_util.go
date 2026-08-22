package server

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// saveUploadTo 在指定目录内安全地保存上传文件。
func saveUploadTo(dir, name string, src io.Reader) (string, error) {
	// 防目录穿越：只取文件名部分
	name = filepath.Base(filepath.ToSlash(name))
	if name == "." || name == ".." || name == "/" {
		return "", fmt.Errorf("invalid file name")
	}
	dst := filepath.Join(dir, name)
	f, err := os.Create(dst)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := io.Copy(f, src); err != nil {
		return "", err
	}
	return dst, nil
}
