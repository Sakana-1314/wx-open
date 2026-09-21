package wxcrypt

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/xml"
	"sort"
	"strings"
	"testing"
)

// 官方测试向量：全部取自官方示例代码与官方文档，逐字节原样抄录。
//   - 向量 A：Java WXBizMsgCryptTest.testAesEncrypt / testAesEncrypt2（固定 randomStr 的加密）
//   - 向量 B：Java WXBizMsgCryptTest.testVerifyUrl（URL 校验）
//   - 向量 C：Python Sample.py（解密，明文含换行，不得 trim）
//   - 向量 D：C++ Sample.cpp（解密）
//   - 向量 E：官方文档「消息推送」的第三方平台 component 端到端示例（43 个 A 的边界 Key）
const (
	vectorAKey      = "abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG"
	vectorAToken    = "pamtest"
	vectorAAppID    = "wxb11529c136998cb6"
	vectorARandom   = "aaaabbbbccccdddd"
	vectorAReplyMsg = "我是中文abcd123"
	vectorACipher   = "jn1L23DB+6ELqJ+6bruv21Y6MD7KeIfP82D6gU39rmkgczbWwt5+3bnyg5K55bgVtVzd832WzZGMhkP72vVOfg=="

	vectorAReplyMsg2 = `<xml><ToUserName><![CDATA[oia2Tj我是中文jewbmiOUlr6X-1crbLOvLw]]></ToUserName><FromUserName><![CDATA[gh_7f083739789a]]></FromUserName><CreateTime>1407743423</CreateTime><MsgType><![CDATA[video]]></MsgType><Video><MediaId><![CDATA[eYJ1MbwPRJtOvIEabaxHs7TX2D-HV71s79GUxqdUkjm6Gs2Ed1KF3ulAOA9H1xG0]]></MediaId><Title><![CDATA[testCallBackReplyVideo]]></Title><Description><![CDATA[testCallBackReplyVideo]]></Description></Video></xml>`
	vectorACipher2   = "jn1L23DB+6ELqJ+6bruv23M2GmYfkv0xBh2h+XTBOKVKcgDFHle6gqcZ1cZrk3e1qjPQ1F4RsLWzQRG9udbKWesxlkupqcEcW7ZQweImX9+wLMa0GaUzpkycA8+IamDBxn5loLgZpnS7fVAbExOkK5DYHBmv5tptA9tklE/fTIILHR8HLXa5nQvFb3tYPKAlHF3rtTeayNf0QuM+UW/wM9enGIDIJHF7CLHiDNAYxr+r+OrJCmPQyTy8cVWlu9iSvOHPT/77bZqJucQHQ04sq7KZI27OcqpQNSto2OdHCoTccjggX5Z9Mma0nMJBU+jLKJ38YB1fBIz+vBzsYjrTmFQ44YfeEuZ+xRTQwr92vhA9OxchWVINGC50qE/6lmkwWTwGX9wtQpsJKhP+oS7rvTY8+VdzETdfakjkwQ5/Xka042OlUb1/slTwo4RscuQ+RdxSGvDahxAJ6+EAjLt9d8igHngxIbf6YyqqROxuxqIeIch3CssH/LqRs+iAcILvApYZckqmA7FNERspKA5f8GoJ9sv8xmGvZ9Yrf57cExWtnX8aCMMaBropU/1k+hKP5LVdzbWCG0hGwx/dQudYR/eXp3P0XxjlFiy+9DMlaFExWUZQDajPkdPrEeOwofJb"

	vectorBToken     = "QDG6eK"
	vectorBKey       = "jWmYm7qr5nMoAUwZRjGtBxmz3KA1tkAj3ykkR6q2B2C"
	vectorBAppID     = "wx5823bf96d3bd56c7"
	vectorBMsgSig    = "5c45ff5e21c57e6ad56bac8758b79b1d9ac89fd3"
	vectorBTimestamp = "1409659589"
	vectorBNonce     = "263014780"
	vectorBechoStr   = "P9nAzCzyDtyTWESHep1vC5X9xho/qYX3Zpb4yKa9SKld1DsH3Iyt3tP3zNdtp+4RPcs8TgAE7OaBO+FZXvnaqQ=="
	vectorBexpect    = "1616140317555161061"

	vectorCToken     = "spamtest"
	vectorCTimestamp = "1409735669"
	vectorCNonce     = "1320562132"
	vectorCAppID     = "wx2c2769f8efd9abc2"
	vectorCMsgSig    = "5d197aaffba7e9b25a30732f161a50dee96bd5fa"
	vectorCBody      = `<xml><ToUserName><![CDATA[gh_10f6c3c3ac5a]]></ToUserName><FromUserName><![CDATA[oyORnuP8q7ou2gfYjqLzSIWZf0rs]]></FromUserName><CreateTime>1409735668</CreateTime><MsgType><![CDATA[text]]></MsgType><Content><![CDATA[abcdteT]]></Content><MsgId>6054768590064713728</MsgId><Encrypt><![CDATA[hyzAe4OzmOMbd6TvGdIOO6uBmdJoD0Fk53REIHvxYtJlE2B655HuD0m8KUePWB3+LrPXo87wzQ1QLvbeUgmBM4x6F8PGHQHFVAFmOD2LdJF9FrXpbUAh0B5GIItb52sn896wVsMSHGuPE328HnRGBcrS7C41IzDWyWNlZkyyXwon8T332jisa+h6tEDYsVticbSnyU8dKOIbgU6ux5VTjg3yt+WGzjlpKn6NPhRjpA912xMezR4kw6KWwMrCVKSVCZciVGCgavjIQ6X8tCOp3yZbGpy0VxpAe+77TszTfRd5RJSVO/HTnifJpXgCSUdUue1v6h0EIBYYI1BD1DlD+C0CR8e6OewpusjZ4uBl9FyJvnhvQl+q5rv1ixrcpCumEPo5MJSgM9ehVsNPfUM669WuMyVWQLCzpu9GhglF2PE=]]></Encrypt></xml>`
	vectorCExpect    = "<xml><ToUserName><![CDATA[gh_10f6c3c3ac5a]]></ToUserName>\n<FromUserName><![CDATA[oyORnuP8q7ou2gfYjqLzSIWZf0rs]]></FromUserName>\n<CreateTime>1409735668</CreateTime>\n<MsgType><![CDATA[text]]></MsgType>\n<Content><![CDATA[abcdteT]]></Content>\n<MsgId>6054768590064713728</MsgId>\n</xml>"

	vectorDToken     = "spamtest"
	vectorDTimestamp = "1410349438"
	vectorDNonce     = "298025754"
	vectorDAppID     = "wx2c2769f8efd9abc2"
	vectorDMsgSig    = "003fee52ecc56afb46c00b5c7721be87860ce785"
	vectorDCipher    = "mfBCs65c67CeJw22u4VT2TD73q5H06+ocrAIxswCaeZ/d/Lw0msSZFHY0teqgSYiI1zR2gD2DKrB3TIrmX/liNSDrGqS8jSI/WPeKB5VPr7Ezr7gomZAyGCwJSgT1TRFWPfONGJMxuj2nk4faTuspAuVIFQ6SHwZuJBZC7mcJp7Cgr9cUhATQWDbOPaE7ukZBTV2YqyzH+UI2AK+J1S47cE79k1RX8t0hcTz/O0hlK8DGXKnvYv88qKQcI7z4iaajqHfRVZKBNyOODabs+It+ZfM3dWTeFcPgDbGtIEnpt/EDtuuA/zMvtkaKdHdswPnVZQ+xdwbYr3ldGvfT8HlEYEgkgKaThxTFobVlwzu2ZkXCjicbP3xdr15Iq48ObgzPpqYuZ3IEoyggZDKClquk0u0orMck4GTF/XyE8yGzc4="

	vectorEToken      = "AAAAA"
	vectorEAppID      = "wx134c8103faa5a59e"
	vectorETimestamp  = "1715943329"
	vectorENonce      = "1590219412"
	vectorEMsgSig     = "6c12a4205838198b8fa631b3220723bb07f1015c"
	vectorEBody       = `<xml><ToUserName><![CDATA[gh_97417a04a28d]]></ToUserName><Encrypt><![CDATA[D7yzvUNAL930rd28wf21s4hvXhz0L6Uit/p2Di6C5DHyYGpEgdBRnKjBec34JwoQXicwaZC7fOVihW80F4VtsdvE//1vr7oAbqjDv8KenVp+ajKYpJnyQ4zMRhIC+a31fCVOMC03FfzV/QuC94kBP55a+Za3sJgvAn+ZbsNqZI5DkyuzkhQN8OBqCzFhizGmy0xpM0MEA4agpvE+RuNO1rhHTtuJB5yltw1FiYzecSXJ+y/D2r81VkRn2eYjh2ltsoyfbDR7Is6ookXIFxTfyeNyeHxMeT4KN5WpCDmSbTcUYrbBUlkGLJ9n/rU8YOywma6G7aTb7KZKOqCxgfoUlYEZPk4FUL/TK7ShriFDMVCLyjQJ15Ob++agDWtcxhfUfe6HIoIpRW8mKNBQiY/Jd1svvuskA1wLef1RTtzKfMpagSCX/laZINmdnX4zrF6kIaR9P4xQrWXbsTFHTYe0Mg==]]></Encrypt></xml>`
	vectorEReceiveID  = "wx134c8103faa5a59e"
	vectorEMsgLen     = 292
	vectorEReplyMsg   = `<xml><demo_resp>[CDATA[good luck]]</demo_resp></xml>`
	vectorERandom     = "999951349e8ee746"
	vectorEReplyCiph  = "hE8R6mGXHkJJjU72KxzKUd1GEkKJaEZq7vRL8XgK3o+00k8JGq6+pZJUIlTSyhsX+bxIBQ72g3GyvDdIZcr6+3HAZbSvPT9t/o11MI7d6WELwqrGd7jMnV0zv3Zc9Nq7"
	vectorEReplySig   = "03e0812039325c2712ef5f0f980fd14c70d6e307"
	vectorEReplyTS    = "1713424427"
	vectorEReplyNonce = "415670741"
)

// vectorEKey 官方示例的 43 个 A（base64 解码后为 32 个 0x00，用于覆盖边界 AESKey）。
var vectorEKey = strings.Repeat("A", 43)

func mustNew(t *testing.T, token, key, appid string) *WXBizMsgCrypt {
	t.Helper()
	c, err := New(token, key, appid, "")
	if err != nil {
		t.Fatalf("构造加解密器失败: %v", err)
	}
	return c
}

// TestOfficialVectorA 加密向量：固定 randomStr 时密文必须与官方一致。
func TestOfficialVectorA(t *testing.T) {
	c := mustNew(t, vectorAToken, vectorAKey, vectorAAppID)

	got, err := c.EncryptPayload(vectorAReplyMsg, vectorAAppID, []byte(vectorARandom))
	if err != nil {
		t.Fatalf("加密失败: %v", err)
	}
	if got != vectorACipher {
		t.Fatalf("向量 A1 不匹配\n期望: %s\n实际: %s", vectorACipher, got)
	}

	got2, err := c.EncryptPayload(vectorAReplyMsg2, vectorAAppID, []byte(vectorARandom))
	if err != nil {
		t.Fatalf("加密失败: %v", err)
	}
	if got2 != vectorACipher2 {
		t.Fatalf("向量 A2 不匹配\n期望: %s\n实际: %s", vectorACipher2, got2)
	}
}

// TestOfficialVectorB URL 校验向量：4 字段签名 + 解密 echoStr + receiveid 校验。
func TestOfficialVectorB(t *testing.T) {
	c := mustNew(t, vectorBToken, vectorBKey, vectorBAppID)

	if sig := c.ComputeSignature(vectorBTimestamp, vectorBNonce, vectorBechoStr); sig != vectorBMsgSig {
		t.Fatalf("向量 B 签名不匹配\n期望: %s\n实际: %s", vectorBMsgSig, sig)
	}
	plain, err := c.VerifyURL(vectorBMsgSig, vectorBTimestamp, vectorBNonce, vectorBechoStr)
	if err != nil {
		t.Fatalf("URL 校验失败: %v", err)
	}
	if plain != vectorBexpect {
		t.Fatalf("echoStr 明文不匹配\n期望: %s\n实际: %s", vectorBexpect, plain)
	}
}

// TestOfficialVectorC 解密向量（明文含换行，必须逐字节相等，不得 trim）。
func TestOfficialVectorC(t *testing.T) {
	c := mustNew(t, vectorCToken, vectorAKey, vectorCAppID)

	msg, receiveID, err := c.DecryptMsg(vectorCMsgSig, vectorCTimestamp, vectorCNonce, vectorCBody, vectorCAppID)
	if err != nil {
		t.Fatalf("解密失败: %v", err)
	}
	if msg != vectorCExpect {
		t.Fatalf("向量 C 明文不匹配\n期望: %q\n实际: %q", vectorCExpect, msg)
	}
	if receiveID != vectorCAppID {
		t.Fatalf("向量 C receiveid 不匹配: %s", receiveID)
	}
}

// TestOfficialVectorD 解密向量（C++ 示例，同一套 Key 与 appid）。
func TestOfficialVectorD(t *testing.T) {
	c := mustNew(t, vectorDToken, vectorAKey, vectorDAppID)
	body := `<xml><ToUserName><![CDATA[gh_10f6c3c3ac5a]]></ToUserName><Encrypt><![CDATA[` + vectorDCipher + `]]></Encrypt></xml>`

	msg, receiveID, err := c.DecryptMsg(vectorDMsgSig, vectorDTimestamp, vectorDNonce, body, vectorDAppID)
	if err != nil {
		t.Fatalf("解密失败: %v", err)
	}
	if receiveID != vectorDAppID {
		t.Fatalf("向量 D receiveid 不匹配: %s", receiveID)
	}
	if !strings.Contains(msg, "<ToUserName><![CDATA[gh_10f6c3c3ac5a]]></ToUserName>") {
		t.Fatalf("向量 D 明文内容异常: %q", msg)
	}
}

// TestOfficialVectorE 第三方平台 component 端到端向量（含 43 个 A 的边界 Key，AESKey 全 0）。
func TestOfficialVectorE(t *testing.T) {
	c := mustNew(t, vectorEToken, vectorEKey, vectorEAppID)

	if sig := c.ComputeSignature(vectorETimestamp, vectorENonce, decodeEncryptOf(t, vectorEBody)); sig != vectorEMsgSig {
		t.Fatalf("向量 E 签名不匹配\n期望: %s\n实际: %s", vectorEMsgSig, sig)
	}

	msg, receiveID, err := c.DecryptMsg(vectorEMsgSig, vectorETimestamp, vectorENonce, vectorEBody, vectorEAppID)
	if err != nil {
		t.Fatalf("向量 E 解密失败: %v", err)
	}
	if len(msg) != vectorEMsgLen {
		t.Fatalf("向量 E 明文长度期望 %d，实际 %d", vectorEMsgLen, len(msg))
	}
	if receiveID != vectorEReceiveID {
		t.Fatalf("向量 E receiveid 期望 %s，实际 %s", vectorEReceiveID, receiveID)
	}
	if !strings.Contains(msg, "<Event><![CDATA[debug_demo]]></Event>") {
		t.Fatalf("向量 E 明文缺少 debug_demo 事件: %q", msg)
	}

	// 回包加密向量：random 与明文原样取自官方示例（含其渲染缺失的 `<![`）。
	cipher, err := c.EncryptPayload(vectorEReplyMsg, vectorEAppID, []byte(vectorERandom))
	if err != nil {
		t.Fatalf("向量 E 回包加密失败: %v", err)
	}
	if cipher != vectorEReplyCiph {
		t.Fatalf("向量 E 回包密文不匹配\n期望: %s\n实际: %s", vectorEReplyCiph, cipher)
	}
	if sig := c.ComputeSignature(vectorEReplyTS, vectorEReplyNonce, cipher); sig != vectorEReplySig {
		t.Fatalf("向量 E 回包签名不匹配\n期望: %s\n实际: %s", vectorEReplySig, sig)
	}
}

// decodeEncryptOf 从信封 XML 中取出 Encrypt 内容。
func decodeEncryptOf(t *testing.T, body string) string {
	t.Helper()
	env, err := parseEnvelope(body)
	if err != nil {
		t.Fatalf("解析信封失败: %v", err)
	}
	return env.Encrypt
}

// TestIllegalAESKey 非 43 字符的 Key 必须被拒绝（官方 testIllegalAesKey）。
func TestIllegalAESKey(t *testing.T) {
	if _, err := New("token", "abcde", "wx123", ""); !IsCode(err, ErrIllegalAESKey) {
		t.Fatalf("期望 ErrIllegalAESKey，实际 %v", err)
	}
	if _, err := New("", vectorAKey, "wx123", ""); err == nil {
		t.Fatalf("空 Token 应当报错")
	}
}

// TestSignatureMismatch 签名被篡改时必须报 -40001。
func TestSignatureMismatch(t *testing.T) {
	c := mustNew(t, vectorCToken, vectorAKey, vectorCAppID)
	if _, _, err := c.DecryptMsg("deadbeef", vectorCTimestamp, vectorCNonce, vectorCBody, vectorCAppID); !IsCode(err, ErrValidateSignature) {
		t.Fatalf("期望 ErrValidateSignature，实际 %v", err)
	}
}

// TestReceiveIDMismatch receiveid 不匹配时必须报 -40005（代收消息与授权事件分流校验的核心保障）。
func TestReceiveIDMismatch(t *testing.T) {
	c := mustNew(t, vectorCToken, vectorAKey, vectorCAppID)
	if _, _, err := c.DecryptMsg(vectorCMsgSig, vectorCTimestamp, vectorCNonce, vectorCBody, "wx_other_appid"); !IsCode(err, ErrValidateReceiveID) {
		t.Fatalf("期望 ErrValidateReceiveID，实际 %v", err)
	}
}

// TestKeyRotation 当前 Key 解不开时回退上一次 Key。
func TestKeyRotation(t *testing.T) {
	// 用旧 Key 加密，然后以「新 Key + 旧 Key」构造，应能解开。
	oldCipher := mustNew(t, vectorCToken, vectorAKey, vectorCAppID)
	cipher, err := oldCipher.EncryptPayload("<xml>hi</xml>", vectorCAppID, []byte("1205899eaf019bbd"))
	if err != nil {
		t.Fatalf("加密失败: %v", err)
	}
	rotated, err := New(vectorCToken, vectorBKey, vectorCAppID, vectorAKey)
	if err != nil {
		t.Fatalf("构造失败: %v", err)
	}
	msg, receiveID, err := rotated.DecryptPayload(cipher, vectorCAppID)
	if err != nil {
		t.Fatalf("Key 轮换回退失败: %v", err)
	}
	if msg != "<xml>hi</xml>" || receiveID != vectorCAppID {
		t.Fatalf("Key 轮换结果异常: msg=%q receiveID=%q", msg, receiveID)
	}
}

// TestDecryptGarbage 非法密文必须归类为可识别的本地错误码，而不是 panic 或静默成功。
func TestDecryptGarbage(t *testing.T) {
	c := mustNew(t, vectorCToken, vectorAKey, vectorCAppID)
	if _, _, err := c.DecryptPayload("!!!not-base64!!!", ""); !IsCode(err, ErrDecodeBase64) {
		t.Fatalf("期望 ErrDecodeBase64，实际 %v", err)
	}
	if _, _, err := c.DecryptPayload("YWJj", ""); !IsCode(err, ErrDecryptAES) {
		t.Fatalf("期望 ErrDecryptAES（长度非 16 倍数），实际 %v", err)
	}
	// 16 字节全 0：填充非法
	if _, _, err := c.DecryptPayload("AAAAAAAAAAAAAAAAAAAAAA==", ""); err == nil {
		t.Fatalf("非法填充应当报错")
	}
}

// TestParseEnvelopeRejectsDoctype 对齐官方 2018 版的 XXE 加固。
func TestParseEnvelopeRejectsDoctype(t *testing.T) {
	body := `<!DOCTYPE foo [<!ENTITY xxe SYSTEM "file:///etc/passwd">]><xml><Encrypt><![CDATA[abc]]></Encrypt></xml>`
	if _, err := parseEnvelope(body); !IsCode(err, ErrParseXML) {
		t.Fatalf("期望拒绝 DOCTYPE，实际 %v", err)
	}
}

// TestEncryptMsgRoundTrip 回包加密 XML 可被自身解回。
func TestEncryptMsgRoundTrip(t *testing.T) {
	c := mustNew(t, vectorEToken, vectorEKey, vectorEAppID)
	xmlStr, err := c.EncryptMsg("hello 微信", vectorETimestamp, vectorENonce, vectorEAppID)
	if err != nil {
		t.Fatalf("生成回包失败: %v", err)
	}
	var reply replyEnvelope
	if err := xml.Unmarshal([]byte(xmlStr), &reply); err != nil {
		t.Fatalf("解析回包失败: %v", err)
	}
	// 回包信封只含 Encrypt/MsgSignature/TimeStamp/Nonce，包成请求信封后解回。
	reqBody := `<xml><ToUserName><![CDATA[gh_test]]></ToUserName><Encrypt><![CDATA[` + reply.Encrypt + `]]></Encrypt></xml>`
	msg, receiveID, err := c.DecryptMsg(reply.MsgSignature, reply.TimeStamp, reply.Nonce, reqBody, vectorEAppID)
	if err != nil {
		t.Fatalf("解回失败: %v", err)
	}
	if msg != "hello 微信" || receiveID != vectorEAppID {
		t.Fatalf("往返结果异常: msg=%q receiveID=%q", msg, receiveID)
	}
}

// TestVerifyPlainURL 兼容经典明文 echostr 的 3 字段校验。
func TestVerifyPlainURL(t *testing.T) {
	c := mustNew(t, "pamtest", vectorAKey, vectorAAppID)
	// 3 字段签名：字典序排序后拼接再 sha1。
	list := []string{"pamtest", "1409304348", "xxxxxx"}
	sort.Strings(list)
	sum := sha1.Sum([]byte(strings.Join(list, "")))
	sig := hex.EncodeToString(sum[:])

	if !c.VerifyPlainURL(sig, "1409304348", "xxxxxx") {
		t.Fatalf("明文 URL 校验失败")
	}
	if c.VerifyPlainURL("wrong", "1409304348", "xxxxxx") {
		t.Fatalf("错误签名不应通过")
	}
}
