package server

import (
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// nanoAlphabet URL-safe 字符集（nanoid 风格，无易混淆字符）。
const nanoAlphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz_-"

// NanoName 生成 n 位 URL-safe 随机名（crypto/rand 均匀采样），如 16 位 ≈ 96bit 熵，碰撞概率极低。
// 用于上传文件命名避免重名（替代长 hex）。
func NanoName(n int) (string, error) {
	if n <= 0 {
		n = 16
	}
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	// 均匀映射：取 2^8 中可整除部分，避免取模偏差
	const alphabetLen = 64 // len(nanoAlphabet) 恒为 2 的幂，无取模偏差
	out := make([]byte, n)
	for i, b := range buf {
		out[i] = nanoAlphabet[int(b)%alphabetLen]
	}
	return string(out), nil
}

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
