// secretHunterTest.lex — end-to-end test for examples/SecretHunter/secretHunter.lex
//
// Plants known-fake secrets in a temp directory, runs the scanner against it
// via _processExec (--no-git --json), parses the output, and asserts that the
// expected pattern names are detected with the correct severity.
//
// All "secrets" in the fixtures are clearly fake (AKIAEXAMPLE… style) so this
// file itself is safe to commit and triggers no real-secret behaviour.

import "stdlib/assert.lex" as t
import "stdlib/json.lex" as js

let FIXTURE = "/tmp/secret_hunter_fixture"

// ─── Fake-secret fixtures. None of these are real credentials. ───
//
// Each value is split with string concatenation so this test file itself
// won't trip basic substring scans by other tools.

let AWS_KEY     = "AKIA" + "IOSFODNN7EXAMPLE"
let GH_PAT      = "ghp_" + "AAAABBBBCCCCDDDDEEEEFFFFGGGGHHHHIIII"
let GOOGLE_KEY  = "AIza" + "SyDummyDummyDummyDummyDummyDummy123"
let STRIPE_LIVE = "sk_live_" + "01234567890123456789ABCD"
let SLACK_TOKEN = "xoxb-" + "1234567890-abcdefghij-XXXXXXXXXX"
let SENDGRID    = "SG." + "AAAAAAAAAAAAAAAAAAAAAA.BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB"
let PRIVKEY     = "-----BEGIN" + " RSA PRIVATE KEY-----"
let DB_URL      = "postgres://" + "user:hunter2@db.example.com:5432/app"
let JWT         = "eyJ" + "abcdefghijkl.eyJabcdefghijkl.signaturesignature"

// ─── Setup ───
println("== Setup: writing fixtures to " + FIXTURE + " ==")
let _, rerr = _fsRemoveAll(FIXTURE)
let _, merr = _fsMkdirAll(FIXTURE)
if merr != null {
    println("setup failed: " + merr)
    _osExit(1)
}

_fsWrite(FIXTURE + "/aws.go",        "const AWS_ACCESS_KEY = \"" + AWS_KEY + "\"\n")
_fsWrite(FIXTURE + "/github.txt",    "token: " + GH_PAT + "\n")
_fsWrite(FIXTURE + "/google.json",   "google_api_key = " + GOOGLE_KEY + "\n")
_fsWrite(FIXTURE + "/stripe.env",    "STRIPE_KEY=" + STRIPE_LIVE + "\n")
_fsWrite(FIXTURE + "/slack.yml",     "slack_bot_token: " + SLACK_TOKEN + "\n")
_fsWrite(FIXTURE + "/sendgrid.ini",  "key = " + SENDGRID + "\n")
_fsWrite(FIXTURE + "/id_rsa",        PRIVKEY + "\nMIIEpAIBAAKCAQEA...\n-----END RSA PRIVATE KEY-----\n")
_fsWrite(FIXTURE + "/db.cfg",        "DATABASE_URL=" + DB_URL + "\n")
_fsWrite(FIXTURE + "/auth.txt",      "session: " + JWT + "\n")
_fsWrite(FIXTURE + "/clean.txt",     "this file has nothing interesting in it at all\n")

// ─── Run scanner via subprocess ───
println("== Running scanner ==")
let stdout, stderr, code, perr = _processExec("./klex", [
    "tests/examples/SecretHunter/secretHunter.lex",
    FIXTURE,
    "--no-git",
    "--json",
])

if perr != null {
    println("could not run ./klex: " + perr)
    _fsRemoveAll(FIXTURE)
    _osExit(1)
}

t.assertEqual(code, 0)
if code != 0 {
    println("scanner stderr: " + stderr)
}

// ─── Parse and inspect findings ───
let findings, jerr = js.parse(stdout)
t.assertNull(jerr)
t.assertNotNull(findings)

// Expected pattern names → severity
let EXPECTED = {
    "AWS Access Key ID":          "CRITICAL",
    "GitHub Classic PAT":         "CRITICAL",
    "Google API Key":             "CRITICAL",
    "Stripe Live Secret Key":     "CRITICAL",
    "Private Key Block":          "CRITICAL",
    "Slack Token":                "HIGH",
    "SendGrid API Key":           "HIGH",
    "Database URL with credentials": "HIGH",
    "JSON Web Token (JWT)":       "MEDIUM",
}

// Build a set of (name → severity) seen in the findings.
let seen = {}
let i = 0
let n = len(findings)
while i < n {
    let f = findings[i]
    seen[f["patternName"]] = f["severity"]
    i = i + 1
}

// Assert each expected pattern was found with the correct severity.
let expected_names = keys(EXPECTED)
i = 0
let m = len(expected_names)
while i < m {
    let name = expected_names[i]
    let wantSev = EXPECTED[name]
    let gotSev  = seen[name]
    if gotSev == null {
        println("MISSING pattern: " + name)
        t.assertNotNull(gotSev)
    } else {
        t.assertEqual(gotSev, wantSev)
    }
    i = i + 1
}

// Assert clean.txt produced no findings (no file with that path in findings).
let cleanHits = 0
i = 0
while i < n {
    let f = findings[i]
    if f["file"] == FIXTURE + "/clean.txt" {
        cleanHits = cleanHits + 1
    }
    i = i + 1
}
t.assertEqual(cleanHits, 0)

// ─── Teardown ───
_fsRemoveAll(FIXTURE)

t.summary()
