// Package wxcrypt 实现微信开放平台消息加解密（WXBizMsgCrypt 的 Go 移植）。
//
// 协议要点（与官方 5 语言示例一致，逐条对应官方示例代码）：
//   - 签名：sha1(字典序排序后的 token、timestamp、nonce、encrypt 四字段直接拼接)，输出 40 位小写 hex；
//   - AESKey = base64decode(EncodingAESKey + "=")，32 字节；
//   - AES-256-CBC，IV = AESKey 前 16 字节，PKCS#7 填充块大小为 32（**不是 AES 的 16**）；
//   - 明文结构：random(16B) + msg_len(4B，网络字节序/大端) + msg + receiveid；
//   - 第三方平台只允许安全模式（纯密文）与 XML，因此校验必须用 msg_signature，官方明确「不要用 signature 验证」；
//   - receiveid 需分流校验：授权事件接收 URL 用第三方平台 appid，代收授权方消息用授权方 appid；
//   - EncodingAESKey 支持轮换：当前 Key 解密失败时回退上一次 Key，回包沿用解密成功的那个 Key。
//
// 与官方示例相比的三处增强：receiveid 作为形参（而非构造期固定）、新旧 Key 双持、错误码类型化。
// 官方示例的 6 组测试向量见 wxcrypt_test.go，用于锁死上述算法细节。
package wxcrypt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
)

// 本地错误码：与官方示例 errorCode.php / ierror.py / AesException.java 完全一致。
// 注意与微信全局 errcode（40001 等，无负号）是两套体系，勿混用。
const (
	// OK 成功。
	OK = 0
	// ErrValidateSignature 签名验证错误。
	ErrValidateSignature = -40001
	// ErrParseXML XML 解析失败。
	ErrParseXML = -40002
	// ErrComputeSignature 生成签名失败。
	ErrComputeSignature = -40003
	// ErrIllegalAESKey EncodingAESKey 非法（非 43 字符，或 base64 解码后不是 32 字节）。
	ErrIllegalAESKey = -40004
	// ErrValidateReceiveID receiveid（appid）校验失败。
	ErrValidateReceiveID = -40005
	// ErrEncryptAES AES 加密失败。
	ErrEncryptAES = -40006
	// ErrDecryptAES AES 解密失败。
	ErrDecryptAES = -40007
	// ErrIllegalBuffer 解密后的缓冲区非法（填充或长度不合法）。
	ErrIllegalBuffer = -40008
	// ErrEncodeBase64 base64 编码失败。
	ErrEncodeBase64 = -40009
	// ErrDecodeBase64 base64 解码失败。
	ErrDecodeBase64 = -40010
	// ErrGenReturnXML 生成回包 XML 失败。
	ErrGenReturnXML = -40011
)

// Error 加解密本地错误。
type Error struct {
	Code    int
	Message string
}

// Error 实现 error。
func (e *Error) Error() string {
	return fmt.Sprintf("消息加解密失败(%d): %s", e.Code, e.Message)
}

func newError(code int, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// IsCode 判断 err 是否为指定本地错误码。
func IsCode(err error, code int) bool {
	var e *Error
	if errors.As(err, &e) {
		return e.Code == code
	}
	return false
}

// pkcs7BlockSize 官方实现使用密钥字节数（32）作为填充块大小，而非 AES 的 16。
const pkcs7BlockSize = 32

// envelope 回调包体信封：安全模式下 `<ToUserName>` + `<Encrypt>`。
type envelope struct {
	XMLName    xml.Name `xml:"xml"`
	ToUserName string   `xml:"ToUserName"`
	Encrypt    string   `xml:"Encrypt"`
	AgentID    string   `xml:"AgentID"`
}

// replyEnvelope 加密回包信封。
type replyEnvelope struct {
	XMLName      xml.Name `xml:"xml"`
	Encrypt      string   `xml:"Encrypt"`
	MsgSignature string   `xml:"MsgSignature"`
	TimeStamp    string   `xml:"TimeStamp"`
	Nonce        string   `xml:"Nonce"`
}

// WXBizMsgCrypt 消息加解密器。
type WXBizMsgCrypt struct {
	token          string
	componentAppID string
	// keys 依次为「当前 Key」「上一次 Key」，解密时顺序尝试，回包沿用命中的那个。
	keys [][]byte
}

// New 构造加解密器。encodingAESKey 为 43 字符的消息加解密 Key，previous 可为空。
func New(token, encodingAESKey, componentAppID, previousEncodingAESKey string) (*WXBizMsgCrypt, error) {
	if token == "" {
		return nil, newError(ErrIllegalAESKey, "消息校验 Token 不能为空")
	}
	key, err := decodeAESKey(encodingAESKey)
	if err != nil {
		return nil, err
	}
	keys := [][]byte{key}
	if previousEncodingAESKey != "" {
		prev, err := decodeAESKey(previousEncodingAESKey)
		if err != nil {
			return nil, fmt.Errorf("上一次的消息加解密 Key 非法: %w", err)
		}
		keys = append(keys, prev)
	}
	return &WXBizMsgCrypt{token: token, componentAppID: componentAppID, keys: keys}, nil
}

// decodeAESKey 由 43 字符 EncodingAESKey 派生 32 字节 AESKey。
func decodeAESKey(encodingAESKey string) ([]byte, error) {
	if len(encodingAESKey) != 43 {
		return nil, newError(ErrIllegalAESKey, "消息加解密 Key 必须是 43 个字符，当前 %d 个", len(encodingAESKey))
	}
	key, err := base64.StdEncoding.DecodeString(encodingAESKey + "=")
	if err != nil {
		return nil, newError(ErrIllegalAESKey, "消息加解密 Key base64 解码失败: %v", err)
	}
	if len(key) != 32 {
		return nil, newError(ErrIllegalAESKey, "消息加解密 Key 解码后必须是 32 字节，当前 %d 字节", len(key))
	}
	return key, nil
}

// Signature 计算签名：四字段字典序排序后直接拼接再做 sha1，输出小写 hex。
//
// 注意：encrypt 必须是 XML 中取出的原文，不做 URL 解码、不做 trim。
func Signature(token, timestamp, nonce, encrypt string) string {
	list := []string{token, timestamp, nonce, encrypt}
	sort.Strings(list)
	sum := sha1.Sum([]byte(strings.Join(list, "")))
	return hex.EncodeToString(sum[:])
}

// ComputeSignature 用实例持有的 token 计算签名。
func (c *WXBizMsgCrypt) ComputeSignature(timestamp, nonce, encrypt string) string {
	return Signature(c.token, timestamp, nonce, encrypt)
}

// VerifyURL 校验首次接入 URL（GET 校验）。
//
// 第三方平台只允许安全模式，因此 echoStr 是 AES 密文：先用 4 字段签名（第 4 个为 echoStr）
// 校验来源，再解密 echoStr 并校验 receiveid（第三方平台 appid），返回解密后的明文。
func (c *WXBizMsgCrypt) VerifyURL(msgSignature, timestamp, nonce, echoStr string) (string, error) {
	if !signatureEqual(c.ComputeSignature(timestamp, nonce, echoStr), msgSignature) {
		return "", newError(ErrValidateSignature, "URL 校验签名不匹配")
	}
	msg, _, err := c.DecryptPayload(echoStr, c.componentAppID)
	if err != nil {
		return "", err
	}
	return msg, nil
}

// VerifyPlainURL 兼容经典（明文 echoStr）URL 校验：使用 3 字段签名，不含 echoStr。
func (c *WXBizMsgCrypt) VerifyPlainURL(signature, timestamp, nonce string) bool {
	list := []string{c.token, timestamp, nonce}
	sort.Strings(list)
	sum := sha1.Sum([]byte(strings.Join(list, "")))
	return signatureEqual(hex.EncodeToString(sum[:]), signature)
}

// DecryptMsg 校验并解密回调包体。
//
// expectedReceiveID：授权事件接收 URL 传第三方平台 appid；代收授权方消息的 URL 传该授权方 appid。
// 传空字符串表示不校验（不推荐，仅用于排障）。
func (c *WXBizMsgCrypt) DecryptMsg(msgSignature, timestamp, nonce, postData, expectedReceiveID string) (string, string, error) {
	env, err := parseEnvelope(postData)
	if err != nil {
		return "", "", err
	}
	if !signatureEqual(c.ComputeSignature(timestamp, nonce, env.Encrypt), msgSignature) {
		return "", "", newError(ErrValidateSignature, "消息签名不匹配（注意必须校验 msg_signature，而非 signature）")
	}
	return c.DecryptPayload(env.Encrypt, expectedReceiveID)
}

// DecryptPayload 解密 base64 密文，返回明文与其中的 receiveid。
func (c *WXBizMsgCrypt) DecryptPayload(encryptB64, expectedReceiveID string) (string, string, error) {
	raw, err := base64.StdEncoding.DecodeString(encryptB64)
	if err != nil {
		return "", "", newError(ErrDecodeBase64, "密文 base64 解码失败: %v", err)
	}
	var lastErr error
	for _, key := range c.keys {
		msg, receiveID, err := decryptWithKey(key, raw, expectedReceiveID)
		if err == nil {
			return msg, receiveID, nil
		}
		lastErr = err
	}
	return "", "", lastErr
}

// decryptWithKey 用单个 AESKey 完成解密与校验。
func decryptWithKey(key, raw []byte, expectedReceiveID string) (string, string, error) {
	if len(raw) == 0 || len(raw)%aes.BlockSize != 0 {
		return "", "", newError(ErrDecryptAES, "密文长度 %d 不是 16 的整数倍", len(raw))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", "", newError(ErrDecryptAES, "构造 AES 失败: %v", err)
	}
	plain := make([]byte, len(raw))
	cipher.NewCBCDecrypter(block, key[:aes.BlockSize]).CryptBlocks(plain, raw)

	plain, err = pkcs7Unpad(plain)
	if err != nil {
		return "", "", err
	}
	if len(plain) < 20 {
		return "", "", newError(ErrIllegalBuffer, "解密后长度 %d 小于最小长度 20", len(plain))
	}
	msgLen := int(binary.BigEndian.Uint32(plain[16:20]))
	if msgLen < 0 || 20+msgLen > len(plain) {
		return "", "", newError(ErrIllegalBuffer, "明文声明的长度 %d 超出实际缓冲区 %d", msgLen, len(plain))
	}
	msg := string(plain[20 : 20+msgLen])
	receiveID := string(plain[20+msgLen:])
	if expectedReceiveID != "" && receiveID != expectedReceiveID {
		return "", "", newError(ErrValidateReceiveID,
			"receiveid 校验失败：期望 %s，实际 %s", expectedReceiveID, receiveID)
	}
	return msg, receiveID, nil
}

// EncryptMsg 生成加密回包 XML（Encrypt / MsgSignature / TimeStamp / Nonce）。
//
// 第三方平台绝大多数回调只需返回明文 success，无需本方法；保留它是为了完整实现协议
// 与「全网发布检测」中需要加密回包的场景。
func (c *WXBizMsgCrypt) EncryptMsg(replyMsg, timestamp, nonce, receiveID string) (string, error) {
	random16 := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, random16); err != nil {
		return "", newError(ErrEncryptAES, "生成随机串失败: %v", err)
	}
	encrypt, err := c.EncryptPayload(replyMsg, receiveID, random16)
	if err != nil {
		return "", err
	}
	out, err := xml.Marshal(replyEnvelope{
		Encrypt:      encrypt,
		MsgSignature: c.ComputeSignature(timestamp, nonce, encrypt),
		TimeStamp:    timestamp,
		Nonce:        nonce,
	})
	if err != nil {
		return "", newError(ErrGenReturnXML, "生成回包 XML 失败: %v", err)
	}
	return string(out), nil
}

// EncryptPayload 加密明文并返回 base64 密文。
//
// random16 必须是 16 字节（官方各语言示例均取 16 个 [A-Za-z0-9] 字符）；
// 显式传入是为了让单测能够复现官方向量（否则密文每次都不同）。
func (c *WXBizMsgCrypt) EncryptPayload(msg, receiveID string, random16 []byte) (string, error) {
	if len(random16) != 16 {
		return "", newError(ErrEncryptAES, "随机串必须是 16 字节，当前 %d 字节", len(random16))
	}
	msgBytes := []byte(msg)
	full := make([]byte, 0, 16+4+len(msgBytes)+len(receiveID))
	full = append(full, random16...)
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(msgBytes)))
	full = append(full, lenBuf[:]...)
	full = append(full, msgBytes...)
	full = append(full, receiveID...)

	padded := pkcs7Pad(full)
	block, err := aes.NewCipher(c.keys[0])
	if err != nil {
		return "", newError(ErrEncryptAES, "构造 AES 失败: %v", err)
	}
	ct := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, c.keys[0][:aes.BlockSize]).CryptBlocks(ct, padded)
	return base64.StdEncoding.EncodeToString(ct), nil
}

// pkcs7Pad 按官方语义填充：不足 32 字节的整数倍时补 (32 - n%32) 个该值字节；
// 已对齐时补满 32 个 0x20。
func pkcs7Pad(data []byte) []byte {
	pad := pkcs7BlockSize - len(data)%pkcs7BlockSize
	if pad == 0 {
		pad = pkcs7BlockSize
	}
	out := make([]byte, len(data)+pad)
	copy(out, data)
	for i := len(data); i < len(out); i++ {
		out[i] = byte(pad)
	}
	return out
}

// pkcs7Unpad 去除填充。采用官方 C++ 版本的严格语义（Java/PHP 会静默宽容，Python 有越界缺陷），
// 非法填充一律返回 -40008。
func pkcs7Unpad(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, newError(ErrIllegalBuffer, "解密结果为空")
	}
	pad := int(data[len(data)-1])
	if pad < 1 || pad > pkcs7BlockSize {
		return nil, newError(ErrIllegalBuffer, "填充长度 %d 不合法（应在 1-32）", pad)
	}
	if len(data)-pad <= 0 {
		return nil, newError(ErrIllegalBuffer, "去除填充后长度 %d 非法", len(data)-pad)
	}
	for _, b := range data[len(data)-pad:] {
		if int(b) != pad {
			return nil, newError(ErrIllegalBuffer, "填充字节内容不一致")
		}
	}
	return data[:len(data)-pad], nil
}

// parseEnvelope 解析回调 XML，显式拒绝 DOCTYPE（对齐官方 2018 版 XXE 加固）。
func parseEnvelope(postData string) (*envelope, error) {
	if strings.Contains(strings.ToUpper(postData), "<!DOCTYPE") {
		return nil, newError(ErrParseXML, "回调 XML 含 DOCTYPE，已按安全策略拒绝")
	}
	var env envelope
	if err := xml.Unmarshal([]byte(postData), &env); err != nil {
		return nil, newError(ErrParseXML, "解析回调 XML 失败: %v", err)
	}
	if env.Encrypt == "" {
		return nil, newError(ErrParseXML, "回调 XML 缺少 Encrypt 字段")
	}
	return &env, nil
}

// signatureEqual 常数时间比较签名。
func signatureEqual(expected, actual string) bool {
	return hmac.Equal([]byte(expected), []byte(actual))
}
