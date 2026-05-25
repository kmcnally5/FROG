import "stdlib/crypto.lex" as crypto

println("=== CRYPTO TEST ===")
println("")

// Test 1: SHA256 hashing
println("Test 1: SHA256 hashing")
let data = "hello world"
let hash = crypto.sha256(data)
println("Input:", data)
println("SHA256:", hash)
println("Length (should be 64):", len(hash))
println("Consistent:", crypto.sha256(data) == hash)
println("")

// Test 2: SHA512 hashing
println("Test 2: SHA512 hashing")
let hash512 = crypto.sha512(data)
println("SHA512 length (should be 128):", len(hash512))
println("SHA512 != SHA256:", hash512 != hash)
println("")

// Test 3: MD5 (legacy)
println("Test 3: MD5 hashing (legacy)")
let md5hash = crypto.md5(data)
println("MD5 length (should be 32):", len(md5hash))
println("")

// Test 4: HMAC-SHA256
println("Test 4: HMAC-SHA256")
let secret = "my_secret_key"
let message = "important message"
let hmac1 = crypto.hmac(secret, message)
let hmac2 = crypto.hmac(secret, message)
let hmac3 = crypto.hmac(secret, "different message")
println("HMAC consistent:", hmac1 == hmac2)
println("Different message has different HMAC:", hmac1 != hmac3)
println("HMAC length (should be 64):", len(hmac1))
println("")

// Test 5: HMAC-SHA512
println("Test 5: HMAC-SHA512")
let hmac512 = crypto.hmacSha512(secret, message)
println("HMAC-SHA512 length (should be 128):", len(hmac512))
println("Different from HMAC-SHA256:", hmac512 != hmac1)
println("")

// Test 6: Password hashing
println("Test 6: Password hashing")
let password = "super_secret_password_123"
let hash_result, err = crypto.hashPassword(password)
println("Hash generated:", err == null)
println("Hash is not empty:", len(hash_result) > 0)
println("Hash looks like bcrypt:", crypto.isValidHash(hash_result, "bcrypt"))
println("")

// Test 7: Password verification
println("Test 7: Password verification")
let matches, err = crypto.verifyPassword(password, hash_result)
println("Correct password matches:", matches == true)
println("No error:", err == null)
println("")

// Test 8: Password verification with wrong password
println("Test 8: Wrong password")
let wrong_matches, err = crypto.verifyPassword("wrong_password", hash_result)
println("Wrong password doesn't match:", wrong_matches == false)
println("No error (mismatch is not an error):", err == null)
println("")

// Test 9: Random token generation
println("Test 9: Random token generation")
let token1, err = crypto.randomToken(16)
println("Token generated:", err == null)
println("Token length (should be 32 for 16 bytes):", len(token1))
let token2, _ = crypto.randomToken(16)
println("Tokens are different:", token1 != token2)
println("")

// Test 10: Random hex alias
println("Test 10: Random hex generation")
let hex_token, err = crypto.randomHex(32)
println("Hex token generated:", err == null)
println("Hex token length (should be 64 for 32 bytes):", len(hex_token))
println("")

// Test 11: Constant-time comparison
println("Test 11: Constant-time comparison")
let token = "abc123xyz"
let matches_ct = crypto.areEqual(token, token)
let no_match_ct = crypto.areEqual(token, "different")
println("Same tokens match:", matches_ct == true)
println("Different tokens don't match:", no_match_ct == false)
println("")

// Test 12: Hash validation
println("Test 12: Hash validation")
let valid_sha256 = crypto.isValidHash(hash, "sha256")
let invalid_sha256 = crypto.isValidHash("short", "sha256")
let valid_bcrypt = crypto.isValidHash(hash_result, "bcrypt")
let invalid_bcrypt = crypto.isValidHash(hash, "bcrypt")
println("Valid SHA256:", valid_sha256)
println("Invalid SHA256:", invalid_sha256 == false)
println("Valid bcrypt:", valid_bcrypt)
println("Invalid bcrypt:", invalid_bcrypt == false)
println("")

// Test 13: Hash size utility
println("Test 13: Hash size utility")
let size_256 = crypto.hashSize("sha256")
let size_512 = crypto.hashSize("sha512")
let size_md5 = crypto.hashSize("md5")
println("SHA256 size:", size_256, "(should be 64)")
println("SHA512 size:", size_512, "(should be 128)")
println("MD5 size:", size_md5, "(should be 32)")
println("")

// Test 14: Error handling - wrong types
println("Test 14: Error handling")
let result_hash = crypto.hash(123)
println("Hash(123) error:", type(result_hash) == "ERROR")
let result_pwd, err_pwd = crypto.hashPassword(456)
println("hashPassword(456) error:", err_pwd != null)
let result_token, err_token = crypto.randomToken(-5)
println("randomToken(-5) error:", err_token != null)
println("")

// Test 15: Real-world use case - API token validation
println("Test 15: Real-world use case")
// Generate API token
let api_token, _ = crypto.randomToken(32)
// Store in system (simulated)
let stored_token = api_token
// User makes request with token
let request_token = api_token
// Verify using constant-time comparison (safe from timing attacks)
let authenticated = crypto.areEqual(request_token, stored_token)
println("Authentication with correct token:", authenticated)
println("")

println("=== CRYPTO TEST COMPLETE ===")

if crypto.areEqual()