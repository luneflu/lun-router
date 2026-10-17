package zai

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strconv"
)

// signKey is the literal HMAC key embedded in chat.z.ai's frontend bundle.
const signKey = "key-@@@@)))()((9))-xxxx&&&%%%%%"

// signatureBucketMS is the signature rotation window. The frontend derives its
// key from floor(timestamp / 300000), so a signature stays valid for the
// five-minute bucket it was minted in.
const signatureBucketMS = 300000

// Payload builds the canonical string the signature covers. The frontend takes
// Object.entries of {timestamp, requestId, user_id}, sorts by key name and joins
// the entries with "," - which yields the value below.
func Payload(timestamp, requestID, userID string) string {
	return "requestId," + requestID + ",timestamp," + timestamp + ",user_id," + userID
}

// Signature reproduces chat.z.ai's x-signature header:
//
//	message = "requestId,<id>,timestamp,<ts>,user_id,<uid>" + "|" + base64(prompt) + "|" + ts
//	bucket  = floor(ts / 300000)
//	key     = hex(HMAC_SHA256("key-@@@@)))()((9))-xxxx&&&%%%%%", str(bucket)))
//	sig     = hex(HMAC_SHA256(key, message))
//
// The inner digest is used as a hex *string* key, not as raw bytes - that detail
// was confirmed against a captured request, where a raw-bytes variant missed.
func Signature(timestamp, requestID, userID, prompt string) string {
	bucket := bucketFor(timestamp)

	inner := hmac.New(sha256.New, []byte(signKey))
	inner.Write([]byte(bucket))
	key := hex.EncodeToString(inner.Sum(nil))

	msg := Payload(timestamp, requestID, userID) + "|" +
		base64.StdEncoding.EncodeToString([]byte(prompt)) + "|" + timestamp

	outer := hmac.New(sha256.New, []byte(key))
	outer.Write([]byte(msg))
	return hex.EncodeToString(outer.Sum(nil))
}

func bucketFor(timestamp string) string {
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return "0"
	}
	return strconv.FormatInt(ts/signatureBucketMS, 10)
}
