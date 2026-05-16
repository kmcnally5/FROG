// secretHunterTest.lex — end-to-end test for tests/examples/SecretHunter/secretHunter.lex
//
// Plants known-fake secrets in a temp directory, runs the scanner against it
// via _processExec (--no-git --json), parses the output, and asserts that the
// expected pattern names are detected with the correct severity.
//
// All "secrets" in the fixtures are clearly fake (AKIAEXAMPLE… style) so this
// file itself is safe to commit and triggers no real-secret behaviour.

import "stdlib/assert.lex" as t
import "stdlib/json.lex" as js

FIXTURE = "/tmp/secret_hunter_fixture"

// ─── Fake-secret fixtures. None of these are real credentials. ───
//
// Each value is split with string concatenation so this test file itself
// won't trip basic substring scans by other tools.

AWS_KEY     = "AKIA" + "IOSFODNN7EXAMPLE"
GH_PAT      = "ghp_" + "AAAABBBBCCCCDDDDEEEEFFFFGGGGHHHHIIII"
GOOGLE_KEY  = "AIza" + "SyDummyDummyDummyDummyDummyDummy123"
STRIPE_LIVE = "sk_live_" + "01234567890123456789ABCD"
SLACK_TOKEN = "xoxb-" + "1234567890-abcdefghij-XXXXXXXXXX"
SENDGRID    = "SG." + "AAAAAAAAAAAAAAAAAAAAAA.BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB"
PRIVKEY     = "-----BEGIN" + " RSA PRIVATE KEY-----"
DB_URL      = "postgres://" + "user:hunter2@db.example.com:5432/app"
JWT         = "eyJ" + "abcdefghijkl.eyJabcdefghijkl.signaturesignature"

// ─── Setup ───
println("== Setup: writing fixtures to " + FIXTURE + " ==")
_, rerr = _fsRemoveAll(FIXTURE)
_, merr = _fsMkdirAll(FIXTURE)
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
stdout, stderr, code, perr = _processExec("./klex", [
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
findings, jerr = js.parse(stdout)
t.assertNull(jerr)
t.assertNotNull(findings)

// Expected pattern names → severity
EXPECTED = {
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
seen = {}
i = 0
n = len(findings)
while i < n {
    f = findings[i]
    seen[f["patternName"]] = f["severity"]
    i = i + 1
}

// Assert each expected pattern was found with the correct severity.
expected_names = keys(EXPECTED)
i = 0
m = len(expected_names)
while i < m {
    name = expected_names[i]
    wantSev = EXPECTED[name]
    gotSev  = seen[name]
    if gotSev == null {
        println("MISSING pattern: " + name)
        t.assertNotNull(gotSev)
    } else {
        t.assertEqual(gotSev, wantSev)
    }
    i = i + 1
}

// Assert clean.txt produced no findings (no file with that path in findings).
cleanHits = 0
i = 0
while i < n {
    f = findings[i]
    if f["file"] == FIXTURE + "/clean.txt" {
        cleanHits = cleanHits + 1
    }
    i = i + 1
}
t.assertEqual(cleanHits, 0)

// ─── Teardown ───
_fsRemoveAll(FIXTURE)

t.summary()
