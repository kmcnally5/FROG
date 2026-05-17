// yaraTest.lex — test YARA integration for SecretHunter

import "tests/examples/SecretHunter/secretHunterLib.lex" as sh

// Start YARA bridge
yaraB, err = sh.startYaraBridge("tests/examples/SecretHunter/secrets.yar")
if err != null {
    println("YARA bridge failed: {err.message}")
    return
}
println("✓ YARA bridge started and rules loaded")

// Create a test file with embedded fake credentials
writeFile("/tmp/sh_yara_test.txt", `
# Config file (test — all values are fake)
AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE
AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
GITHUB_TOKEN=ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdef1234
STRIPE_KEY=sk_live_TESTKEY123456789012345678901234
DB_URL=postgres://admin:supersecret@prod-db.company.com:5432/myapp
JWT_SECRET=my-super-secret-jwt-signing-key-do-not-share
SLACK_TOKEN=xoxb-123456789-ABCDEFGHIJKLMNOPQRSTUVWXYZ
`)

// Run YARA scan on test file using the standalone bridge
files = ["/tmp/sh_yara_test.txt"]
matches, err = bridgeCall(yaraB, "scan_batch", [files])
if err != null { println("scan error: {err.message}")  return }
findings = makeArray(len(matches))
i = 0
while i < len(matches) {
    hit = matches[i]
    findings[i] = {"patternName": hit["rule"], "severity": hit["severity"], "match": hit["match"]}
    i = i + 1
}

println("\n--- YARA findings ---")
n = len(findings)
i = 0
while i < n {
    f = findings[i]
    sev  = f["severity"]
    rule = f["patternName"]
    m    = f["match"]
    if len(m) > 50 { m = substr(m, 0, 47) + "..." }
    println("[{sev}] {rule}")
    println("       match: {m}")
    i = i + 1
}
println("\n{n} YARA finding(s) in test file")

// Clean up
bridgeClose(yaraB)
println("✓ YARA bridge closed")
