package eval

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/pbkdf2"
)

func init() {
	// _sha256(data) → hex_string
	// Returns SHA256 hash of data as hex string
	Builtins["_sha256"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return &Tuple{Elements: []Object{NULL, &String{Value: "_sha256 expects 1 argument"}}}
		}
		s, ok := args[0].(*String)
		if !ok {
			return &Tuple{Elements: []Object{NULL, &String{Value: fmt.Sprintf("_sha256: argument must be string, got %s", args[0].Type())}}}
		}
		hash := sha256.Sum256([]byte(s.Value))
		return &String{Value: hex.EncodeToString(hash[:])}
	}}

	// _sha512(data) → hex_string
	// Returns SHA512 hash of data as hex string
	Builtins["_sha512"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return &Tuple{Elements: []Object{NULL, &String{Value: "_sha512 expects 1 argument"}}}
		}
		s, ok := args[0].(*String)
		if !ok {
			return &Tuple{Elements: []Object{NULL, &String{Value: fmt.Sprintf("_sha512: argument must be string, got %s", args[0].Type())}}}
		}
		hash := sha512.Sum512([]byte(s.Value))
		return &String{Value: hex.EncodeToString(hash[:])}
	}}

	// _md5(data) → hex_string
	// Returns MD5 hash of data as hex string (DEPRECATED: use SHA256)
	// MD5 is cryptographically broken; use SHA256 for security
	Builtins["_md5"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return &Tuple{Elements: []Object{NULL, &String{Value: "_md5 expects 1 argument"}}}
		}
		s, ok := args[0].(*String)
		if !ok {
			return &Tuple{Elements: []Object{NULL, &String{Value: fmt.Sprintf("_md5: argument must be string, got %s", args[0].Type())}}}
		}
		hash := md5.Sum([]byte(s.Value))
		return &String{Value: hex.EncodeToString(hash[:])}
	}}

	// _hmacSha256(key, data) → hex_string
	// Returns HMAC-SHA256 of data with key as hex string
	Builtins["_hmacSha256"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return &Tuple{Elements: []Object{NULL, &String{Value: "_hmacSha256 expects 2 arguments (key, data)"}}}
		}
		keyObj, ok := args[0].(*String)
		if !ok {
			return &Tuple{Elements: []Object{NULL, &String{Value: fmt.Sprintf("_hmacSha256: key must be string, got %s", args[0].Type())}}}
		}
		dataObj, ok := args[1].(*String)
		if !ok {
			return &Tuple{Elements: []Object{NULL, &String{Value: fmt.Sprintf("_hmacSha256: data must be string, got %s", args[1].Type())}}}
		}
		h := hmac.New(sha256.New, []byte(keyObj.Value))
		h.Write([]byte(dataObj.Value))
		return &String{Value: hex.EncodeToString(h.Sum(nil))}
	}}

	// _hmacSha512(key, data) → hex_string
	// Returns HMAC-SHA512 of data with key as hex string
	Builtins["_hmacSha512"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return &Tuple{Elements: []Object{NULL, &String{Value: "_hmacSha512 expects 2 arguments (key, data)"}}}
		}
		keyObj, ok := args[0].(*String)
		if !ok {
			return &Tuple{Elements: []Object{NULL, &String{Value: fmt.Sprintf("_hmacSha512: key must be string, got %s", args[0].Type())}}}
		}
		dataObj, ok := args[1].(*String)
		if !ok {
			return &Tuple{Elements: []Object{NULL, &String{Value: fmt.Sprintf("_hmacSha512: data must be string, got %s", args[1].Type())}}}
		}
		h := hmac.New(sha512.New, []byte(keyObj.Value))
		h.Write([]byte(dataObj.Value))
		return &String{Value: hex.EncodeToString(h.Sum(nil))}
	}}

	// _bcryptHash(password) → (hash, err)
	// Generates bcrypt hash of password (cost=12, default)
	Builtins["_bcryptHash"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return &Tuple{Elements: []Object{NULL, &String{Value: "_bcryptHash expects 1 argument"}}}
		}
		s, ok := args[0].(*String)
		if !ok {
			return &Tuple{Elements: []Object{NULL, &String{Value: fmt.Sprintf("_bcryptHash: argument must be string, got %s", args[0].Type())}}}
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(s.Value), bcrypt.DefaultCost)
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: fmt.Sprintf("bcrypt error: %s", err.Error())}}}
		}
		return &Tuple{Elements: []Object{&String{Value: string(hash)}, NULL}}
	}}

	// _bcryptVerify(password, hash) → (matches, err)
	// Verifies password against bcrypt hash
	// Returns (true, null) if password matches
	// Returns (false, null) if password doesn't match
	// Returns (false, error) on error
	Builtins["_bcryptVerify"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return &Tuple{Elements: []Object{NULL, &String{Value: "_bcryptVerify expects 2 arguments (password, hash)"}}}
		}
		pwdObj, ok := args[0].(*String)
		if !ok {
			return &Tuple{Elements: []Object{NULL, &String{Value: fmt.Sprintf("_bcryptVerify: password must be string, got %s", args[0].Type())}}}
		}
		hashObj, ok := args[1].(*String)
		if !ok {
			return &Tuple{Elements: []Object{NULL, &String{Value: fmt.Sprintf("_bcryptVerify: hash must be string, got %s", args[1].Type())}}}
		}
		err := bcrypt.CompareHashAndPassword([]byte(hashObj.Value), []byte(pwdObj.Value))
		if err == nil {
			return &Tuple{Elements: []Object{&Boolean{Value: true}, NULL}}
		}
		if err == bcrypt.ErrMismatchedHashAndPassword {
			return &Tuple{Elements: []Object{&Boolean{Value: false}, NULL}}
		}
		return &Tuple{Elements: []Object{&Boolean{Value: false}, &String{Value: fmt.Sprintf("bcrypt error: %s", err.Error())}}}
	}}

	// _randomBytes(length) → (random_hex_string, err)
	// Generates cryptographically secure random bytes
	// Returns as hex string
	Builtins["_randomBytes"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 1 {
			return &Tuple{Elements: []Object{NULL, &String{Value: "_randomBytes expects 1 argument"}}}
		}
		lengthObj, ok := args[0].(*Integer)
		if !ok {
			return &Tuple{Elements: []Object{NULL, &String{Value: fmt.Sprintf("_randomBytes: argument must be integer, got %s", args[0].Type())}}}
		}
		if lengthObj.Value < 0 {
			return &Tuple{Elements: []Object{NULL, &String{Value: "length must be non-negative"}}}
		}
		bytes := make([]byte, lengthObj.Value)
		_, err := rand.Read(bytes)
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: fmt.Sprintf("random generation error: %s", err.Error())}}}
		}
		return &Tuple{Elements: []Object{&String{Value: hex.EncodeToString(bytes)}, NULL}}
	}}

	// _constantTimeEquals(a, b) → boolean
	// Compares two strings in constant time (resistant to timing attacks)
	// Use this for comparing passwords, tokens, signatures
	Builtins["_constantTimeEquals"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return &Boolean{Value: false}
		}
		aObj, ok := args[0].(*String)
		if !ok {
			return &Boolean{Value: false}
		}
		bObj, ok := args[1].(*String)
		if !ok {
			return &Boolean{Value: false}
		}
		return &Boolean{Value: hmac.Equal([]byte(aObj.Value), []byte(bObj.Value))}
	}}

	// _aesEncrypt(plaintext, key) → (ciphertext_hex, error)
	// Encrypts plaintext using AES-256-GCM (authenticated encryption)
	// key: 32-byte key (will be used as-is; derive from password with pbkdf2 if needed)
	// Returns: hex-encoded (nonce + ciphertext + tag) or error
	// Format: 24 hex chars (12 bytes nonce) + ciphertext + tag (always 32 hex chars)
	Builtins["_aesEncrypt"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return &Tuple{Elements: []Object{NULL, &String{Value: "_aesEncrypt expects 2 arguments (plaintext, key)"}}}
		}
		ptObj, ok := args[0].(*String)
		if !ok {
			return &Tuple{Elements: []Object{NULL, &String{Value: fmt.Sprintf("_aesEncrypt: plaintext must be string, got %s", args[0].Type())}}}
		}
		keyObj, ok := args[1].(*String)
		if !ok {
			return &Tuple{Elements: []Object{NULL, &String{Value: fmt.Sprintf("_aesEncrypt: key must be string, got %s", args[1].Type())}}}
		}

		keyBytes := []byte(keyObj.Value)
		// Derive 32-byte key from input if needed
		if len(keyBytes) < 32 {
			// Key too short, use PBKDF2 to derive it
			salt := []byte("frog_broker_salt") // Fixed salt; in production use random per-file
			keyBytes = pbkdf2.Key(keyBytes, salt, 100000, 32, sha256.New)
		} else if len(keyBytes) > 32 {
			keyBytes = keyBytes[:32]
		}

		block, err := aes.NewCipher(keyBytes)
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: fmt.Sprintf("aes cipher error: %s", err.Error())}}}
		}

		gcm, err := cipher.NewGCM(block)
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: fmt.Sprintf("gcm error: %s", err.Error())}}}
		}

		nonce := make([]byte, gcm.NonceSize())
		_, err = rand.Read(nonce)
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: fmt.Sprintf("random nonce error: %s", err.Error())}}}
		}

		ciphertext := gcm.Seal(nonce, nonce, []byte(ptObj.Value), nil)
		return &Tuple{Elements: []Object{&String{Value: hex.EncodeToString(ciphertext)}, NULL}}
	}}

	// _aesDecrypt(ciphertext_hex, key) → (plaintext, error)
	// Decrypts AES-256-GCM ciphertext (expects format from _aesEncrypt)
	// Returns (plaintext, null) on success or (null, error) on failure
	Builtins["_aesDecrypt"] = &Builtin{Fn: func(args []Object) Object {
		if len(args) != 2 {
			return &Tuple{Elements: []Object{NULL, &String{Value: "_aesDecrypt expects 2 arguments (ciphertext_hex, key)"}}}
		}
		ctObj, ok := args[0].(*String)
		if !ok {
			return &Tuple{Elements: []Object{NULL, &String{Value: fmt.Sprintf("_aesDecrypt: ciphertext must be string, got %s", args[0].Type())}}}
		}
		keyObj, ok := args[1].(*String)
		if !ok {
			return &Tuple{Elements: []Object{NULL, &String{Value: fmt.Sprintf("_aesDecrypt: key must be string, got %s", args[1].Type())}}}
		}

		cipherBytes, err := hex.DecodeString(ctObj.Value)
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: fmt.Sprintf("hex decode error: %s", err.Error())}}}
		}

		keyBytes := []byte(keyObj.Value)
		// Derive 32-byte key from input if needed (must match encryption)
		if len(keyBytes) < 32 {
			salt := []byte("frog_broker_salt") // Must match encryption
			keyBytes = pbkdf2.Key(keyBytes, salt, 100000, 32, sha256.New)
		} else if len(keyBytes) > 32 {
			keyBytes = keyBytes[:32]
		}

		block, err := aes.NewCipher(keyBytes)
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: fmt.Sprintf("aes cipher error: %s", err.Error())}}}
		}

		gcm, err := cipher.NewGCM(block)
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: fmt.Sprintf("gcm error: %s", err.Error())}}}
		}

		nonceSize := gcm.NonceSize()
		if len(cipherBytes) < nonceSize {
			return &Tuple{Elements: []Object{NULL, &String{Value: "ciphertext too short"}}}
		}

		nonce, ciphertext := cipherBytes[:nonceSize], cipherBytes[nonceSize:]
		plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
		if err != nil {
			return &Tuple{Elements: []Object{NULL, &String{Value: fmt.Sprintf("decryption failed (corrupted or wrong key): %s", err.Error())}}}
		}

		return &Tuple{Elements: []Object{&String{Value: string(plaintext)}, NULL}}
	}}
}
