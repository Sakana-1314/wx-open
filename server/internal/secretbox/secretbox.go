// Package secretbox 用 AES-256-GCM 加密需要落库的敏感值（当前只有 authorizer_refresh_token）。
//
// 密钥来自 SECRET_ENC_KEY（32 字节 base64）或由 JWT_SECRET 派生，进程启动时一次性注入。
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
)

// Box 加解密封装。
type Box struct {
	aead cipher.AEAD
}

// New 用 32 字节密钥构造 Box。
func New(key []byte) (*Box, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("加密密钥必须是 32 字节，当前 %d 字节", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("构造 AES 失败: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("构造 GCM 失败: %w", err)
	}
	return &Box{aead: aead}, nil
}

// Seal 加密：输出 nonce || ciphertext（含 GCM tag）。
func (b *Box) Seal(plain string) ([]byte, error) {
	if plain == "" {
		return nil, nil
	}
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("生成随机数失败: %w", err)
	}
	return b.aead.Seal(nonce, nonce, []byte(plain), nil), nil
}

// Open 解密 Seal 的输出。
func (b *Box) Open(data []byte) (string, error) {
	if len(data) == 0 {
		return "", nil
	}
	ns := b.aead.NonceSize()
	if len(data) < ns {
		return "", errors.New("密文长度不足")
	}
	plain, err := b.aead.Open(nil, data[:ns], data[ns:], nil)
	if err != nil {
		return "", fmt.Errorf("解密失败（SECRET_ENC_KEY 是否被更换？）: %w", err)
	}
	return string(plain), nil
}

// SealToBase64 加密并 base64 编码（便于日志/导出场景，不用于数据库列）。
func (b *Box) SealToBase64(plain string) (string, error) {
	sealed, err := b.Seal(plain)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(sealed), nil
}
