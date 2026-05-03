// ============================================================================
// crypto.lex — Cryptographic functions
// ============================================================================
//
// Provides hashing, HMAC, password hashing, and secure random token generation.
// Use this for:
//   - Password verification (bcrypt)
//   - Message authentication (HMAC)
//   - Data integrity checking (SHA256)
//   - Secure token generation
//
// CRITICAL: All functions use cryptographically secure algorithms
// - SHA256/SHA512: cryptographic hashes
// - bcrypt: password hashing with work factor (resistant to brute force)
// - HMAC: message authentication codes
// - crypto/rand: cryptographically secure random
//
// Example:
//   password = "my_secret_password"
//   hash, _ = hashPassword(password)     // store hash
//   matches, _ = verifyPassword(password, hash)  // check password

// ============================================================================
// HASHING (for data integrity, NOT passwords)
// ============================================================================

// hash(data) → hex_string
// Computes SHA256 hash of data. Default hash algorithm.
// Use for: data integrity, checksums, fingerprinting
// DO NOT USE for password hashing — use hashPassword() instead
//
// Example:
//   checksum = hash(file_data)
//   if hash(received_data) != checksum {
//       println("data corrupted")
//   }
fn hash(data) {
    if type(data) != "STRING" {
        return error("TYPE_ERROR", "hash expects string, got " + type(data))
    }
    return _sha256(data)  // returns string directly (no error possible)
}

// sha256(data) → hex_string
// Computes SHA256 hash of data
// Returns 64-character hex string (256 bits)
//
// Example:
//   h = sha256("password123")
//   println(h)  // "ef2d127de37b493...[64 chars total]"
fn sha256(data) {
    if type(data) != "STRING" {
        return error("TYPE_ERROR", "sha256 expects string, got " + type(data))
    }
    return _sha256(data)
}

// sha512(data) → hex_string
// Computes SHA512 hash of data
// Returns 128-character hex string (512 bits)
// Use when you need extra security margin over SHA256
//
// Example:
//   h = sha512("data")
fn sha512(data) {
    if type(data) != "STRING" {
        return error("TYPE_ERROR", "sha512 expects string, got " + type(data))
    }
    return _sha512(data)
}

// md5(data) → hex_string
// Computes MD5 hash of data
// WARNING: MD5 is cryptographically broken. Use SHA256 instead.
// Only use MD5 for:
//   - Compatibility with legacy systems
//   - Non-security checksums (like git object IDs)
// NEVER use MD5 for password hashing or security-sensitive applications
//
// Example:
//   // legacy compatibility only
//   h = md5(data)
fn md5(data) {
    if type(data) != "STRING" {
        return error("TYPE_ERROR", "md5 expects string, got " + type(data))
    }
    return _md5(data)
}

// ============================================================================
// HMAC (Message Authentication Codes)
// ============================================================================

// hmac(key, data) → hex_string
// Computes HMAC-SHA256 of data with key. Default HMAC algorithm.
// Use for: verifying message authenticity and integrity
// Both sender and receiver must know the secret key
//
// Example:
//   secret = "shared_secret_key"
//   message = "important data"
//   signature = hmac(secret, message)
//   // Send message + signature to recipient
//   // Recipient verifies: hmac(secret, received_message) == signature
fn hmac(key, data) {
    if type(key) != "STRING" {
        return error("TYPE_ERROR", "hmac: key must be string, got " + type(key))
    }
    if type(data) != "STRING" {
        return error("TYPE_ERROR", "hmac: data must be string, got " + type(data))
    }
    return _hmacSha256(key, data)
}

// hmacSha256(key, data) → hex_string
// Computes HMAC-SHA256 of data with key
// Returns 64-character hex string
//
// Example:
//   sig = hmacSha256("secret", "message")
fn hmacSha256(key, data) {
    if type(key) != "STRING" {
        return error("TYPE_ERROR", "hmacSha256: key must be string, got " + type(key))
    }
    if type(data) != "STRING" {
        return error("TYPE_ERROR", "hmacSha256: data must be string, got " + type(data))
    }
    return _hmacSha256(key, data)
}

// hmacSha512(key, data) → hex_string
// Computes HMAC-SHA512 of data with key
// Returns 128-character hex string
// Use when you need extra security margin over HMAC-SHA256
//
// Example:
//   sig = hmacSha512("secret", "message")
fn hmacSha512(key, data) {
    if type(key) != "STRING" {
        return error("TYPE_ERROR", "hmacSha512: key must be string, got " + type(key))
    }
    if type(data) != "STRING" {
        return error("TYPE_ERROR", "hmacSha512: data must be string, got " + type(data))
    }
    return _hmacSha512(key, data)
}

// ============================================================================
// PASSWORD HASHING (use bcrypt)
// ============================================================================

// hashPassword(password) → (hash, error)
// Hashes a password using bcrypt with cost=12 (default)
// Returns bcrypt hash string or error
// ALWAYS use this for password storage, never use plain SHA256
//
// Example:
//   password = "user_password_123"
//   hash, err = hashPassword(password)
//   if err == null {
//       saveToDatabase({"password_hash": hash})
//   }
fn hashPassword(password) {
    if type(password) != "STRING" {
        return null, error("TYPE_ERROR", "hashPassword expects string, got " + type(password))
    }
    return safe(_bcryptHash, password)
}

// verifyPassword(password, hash) → (matches, error)
// Verifies a password against a bcrypt hash
// Returns (true, null) if password matches
// Returns (false, null) if password doesn't match
// Returns (false, error) on error
//
// Example:
//   stored_hash = getPasswordHashFromDatabase(user_id)
//   matches, err = verifyPassword(user_input, stored_hash)
//   if err == null && matches {
//       loginUser()
//   }
fn verifyPassword(password, hash) {
    if type(password) != "STRING" {
        return null, error("TYPE_ERROR", "verifyPassword: password must be string")
    }
    if type(hash) != "STRING" {
        return null, error("TYPE_ERROR", "verifyPassword: hash must be string")
    }
    return safe(_bcryptVerify, password, hash)
}

// ============================================================================
// SECURE RANDOM TOKEN GENERATION
// ============================================================================

// randomToken(length) → (token, error)
// Generates a cryptographically secure random hex token of specified byte length
// Use for: API tokens, session IDs, security tokens
// Returns hex string (each byte becomes 2 hex characters)
//
// Example:
//   token, err = randomToken(32)  // 64-character hex string
//   if err == null {
//       saveToken(token)
//   }
fn randomToken(length) {
    if type(length) != "INTEGER" {
        return null, error("TYPE_ERROR", "randomToken expects integer, got " + type(length))
    }
    if length < 0 {
        return null, error("INVALID_ARG", "length must be non-negative")
    }
    return safe(_randomBytes, length)
}

// randomHex(byteCount) → (hex_string, error)
// Alias for randomToken. Generates random hex string.
// byteCount: number of random bytes (result will be 2*byteCount hex chars)
//
// Example:
//   h, _ = randomHex(16)  // 32-character hex string (128 bits)
fn randomHex(byteCount) {
    return randomToken(byteCount)
}

// ============================================================================
// CONSTANT-TIME COMPARISON (for security)
// ============================================================================

// areEqual(a, b) → boolean
// Compares two strings in constant time
// Use for comparing sensitive values like passwords, tokens, signatures
// Prevents timing-based attacks where attacker can guess correct characters
// by measuring comparison time
//
// Example:
//   if areEqual(user_token, expected_token) {
//       // both tokens match
//   }
//   // Safe from timing attacks
fn areEqual(a, b) {
    if type(a) != "STRING" {
        return false
    }
    if type(b) != "STRING" {
        return false
    }
    return _constantTimeEquals(a, b)
}

// ============================================================================
// UTILITY FUNCTIONS
// ============================================================================

// isValidHash(hash, type) → boolean
// Checks if a string looks like a valid hash of the given type
// type: "sha256", "sha512", "md5", "bcrypt"
//
// Example:
//   if isValidHash(some_value, "sha256") {
//       // looks like a SHA256 hash (64 hex chars)
//   }
fn isValidHash(hash, hashType) {
    if type(hash) != "STRING" {
        return false
    }
    if type(hashType) != "STRING" {
        return false
    }

    len_h = len(hash)

    if hashType == "sha256" {
        return len_h == 64
    }
    if hashType == "sha512" {
        return len_h == 128
    }
    if hashType == "md5" {
        return len_h == 32
    }
    if hashType == "bcrypt" {
        // bcrypt hashes start with $2a$, $2b$, $2x$, or $2y$
        return startsWith(hash, "$2a$") || startsWith(hash, "$2b$") ||
               startsWith(hash, "$2x$") || startsWith(hash, "$2y$")
    }

    return false
}

// hashSize(hashType) → integer
// Returns the expected size (in hex characters) of a hash of given type
//
// Example:
//   size = hashSize("sha256")  // 64
fn hashSize(hashType) {
    if type(hashType) != "STRING" {
        return 0
    }

    if hashType == "sha256" {
        return 64
    }
    if hashType == "sha512" {
        return 128
    }
    if hashType == "md5" {
        return 32
    }

    return 0
}
