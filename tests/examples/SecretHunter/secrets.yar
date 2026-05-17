/*
 * secrets.yar — YARA rules for SecretHunter
 *
 * Metadata fields used by the bridge:
 *   severity : CRITICAL | HIGH | MEDIUM | LOW
 *   action   : remediation advice returned in findings
 *
 * Note: use `nocase` modifier for case-insensitive matching
 * rather than (?i) inline flags (YARA subset of PCRE).
 */

// ── Cloud Providers ───────────────────────────────────────────────────────────

rule AWS_Access_Key {
    meta:
        severity = "CRITICAL"
        action   = "Rotate this key immediately in the AWS IAM console. Check CloudTrail for unauthorised usage."
    strings:
        $key = /AKIA[0-9A-Z]{16}/
    condition:
        $key
}

rule AWS_Secret_Access_Key {
    meta:
        severity = "CRITICAL"
        action   = "Rotate the AWS access key pair. The secret alone is useless without the ID but treat both as compromised."
    strings:
        $label = /aws_secret_access_key\s*[=:]\s*/ nocase
        $value = /[A-Za-z0-9\/+=]{40}/
    condition:
        $label and $value and @value > @label and @value - @label < 100
}

rule AWS_Session_Token {
    meta:
        severity = "CRITICAL"
        action   = "Rotate the originating long-term credentials and revoke active sessions in IAM."
    strings:
        $label = /aws_session_token\s*[=:"']+\s*/ nocase
        $token = /[A-Za-z0-9\/+=]{100,}/
    condition:
        $label and $token and @token > @label and @token - @label < 20
}

rule Google_API_Key {
    meta:
        severity = "CRITICAL"
        action   = "Rotate this key in Google Cloud Console → APIs & Services → Credentials."
    strings:
        $key = /AIza[0-9A-Za-z\-_]{35}/
    condition:
        $key
}

rule Azure_Storage_Key {
    meta:
        severity = "CRITICAL"
        action   = "Rotate the storage account access key in Azure Portal. Use SAS tokens instead."
    strings:
        $label  = /AccountKey\s*[=:]\s*/ nocase
        $value  = /[A-Za-z0-9+\/]{86}==/
    condition:
        $label and $value
}

// ── Source Control ────────────────────────────────────────────────────────────

rule GitHub_Classic_PAT {
    meta:
        severity = "CRITICAL"
        action   = "Revoke immediately on GitHub → Settings → Developer settings → Personal access tokens. Purge from history with git filter-repo."
    strings:
        $pat = /ghp_[A-Za-z0-9]{36}/
    condition:
        $pat
}

rule GitHub_Fine_Grained_PAT {
    meta:
        severity = "CRITICAL"
        action   = "Revoke immediately on GitHub. Fine-grained tokens have scoped permissions — check the token's access log."
    strings:
        $pat = /github_pat_[A-Za-z0-9_]{82}/
    condition:
        $pat
}

rule GitHub_OAuth_Token {
    meta:
        severity = "CRITICAL"
        action   = "Revoke immediately on GitHub. Re-authorise the OAuth app."
    strings:
        $tok = /gho_[A-Za-z0-9]{36}/
    condition:
        $tok
}

rule GitLab_PAT {
    meta:
        severity = "CRITICAL"
        action   = "Revoke immediately in GitLab → User Settings → Access Tokens."
    strings:
        $pat = /glpat-[A-Za-z0-9\-_]{20}/
    condition:
        $pat
}

// ── Private Keys ──────────────────────────────────────────────────────────────

rule Private_Key_PEM {
    meta:
        severity = "CRITICAL"
        action   = "Remove the private key from the repo and history. Generate a new key pair. Revoke any certificates signed by this key."
    strings:
        $rsa  = "-----BEGIN RSA PRIVATE KEY-----"
        $ec   = "-----BEGIN EC PRIVATE KEY-----"
        $oss  = "-----BEGIN OPENSSH PRIVATE KEY-----"
        $dsa  = "-----BEGIN DSA PRIVATE KEY-----"
        $priv = "-----BEGIN PRIVATE KEY-----"
    condition:
        any of them
}

// ── Payment & Fintech ─────────────────────────────────────────────────────────

rule Stripe_Live_Key {
    meta:
        severity = "CRITICAL"
        action   = "Roll the key in Stripe Dashboard → Developers → API keys immediately. Check logs for unauthorised charges."
    strings:
        $live = /sk_live_[0-9a-zA-Z]{24,}/
    condition:
        $live
}

rule Stripe_Test_Key {
    meta:
        severity = "LOW"
        action   = "Test key — verify this is not a live key and not exposed in production."
    strings:
        $test = /sk_test_[0-9a-zA-Z]{24,}/
    condition:
        $test
}

// ── Communication Services ────────────────────────────────────────────────────

rule Slack_Token {
    meta:
        severity = "HIGH"
        action   = "Revoke the token in Slack app settings → OAuth & Permissions."
    strings:
        $bot  = /xoxb-[0-9A-Za-z\-]{10,}/
        $user = /xoxp-[0-9A-Za-z\-]{10,}/
        $app  = /xoxa-[0-9A-Za-z\-]{10,}/
    condition:
        any of them
}

rule SendGrid_API_Key {
    meta:
        severity = "HIGH"
        action   = "Revoke the key in SendGrid → Settings → API Keys."
    strings:
        $key = /SG\.[A-Za-z0-9_\-]{22}\.[A-Za-z0-9_\-]{43}/
    condition:
        $key
}

// ── Database Credentials ──────────────────────────────────────────────────────

rule Database_URL_With_Credentials {
    meta:
        severity = "HIGH"
        action   = "Rotate the database user password and move credentials to environment variables or a secret manager."
    strings:
        $pg    = /postgres:\/\/[^\s:]+:[^\s@\/]+@/
        $mysql = /mysql:\/\/[^\s:]+:[^\s@\/]+@/
        $mongo = /mongodb:\/\/[^\s:]+:[^\s@\/]+@/
    condition:
        any of them
}

rule MSSQL_Connection_String {
    meta:
        severity = "HIGH"
        action   = "Rotate the database password and store it in an environment variable or secret manager."
    strings:
        $pw1 = /Password=[^;'"\s]{8,}/ nocase
        $pw2 = /PWD=[^;'"\s]{8,}/
    condition:
        any of them
}

// ── Auth Tokens ───────────────────────────────────────────────────────────────

rule JWT_Token {
    meta:
        severity = "MEDIUM"
        action   = "Review the JWT claims. If it grants real access, rotate the signing key and invalidate issued tokens."
    strings:
        $jwt = /eyJ[A-Za-z0-9_\-]{10,}\.eyJ[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,}/
    condition:
        $jwt
}

rule Bearer_Token {
    meta:
        severity = "HIGH"
        action   = "A bearer token is hardcoded. Move to environment variables or a secret manager."
    strings:
        $bearer = /bearer\s+[A-Za-z0-9_\-]{20,}/  nocase
    condition:
        $bearer
}

rule Hardcoded_Password {
    meta:
        severity = "HIGH"
        action   = "Move the hardcoded password to an environment variable or secret manager."
    strings:
        $pw1 = /password\s*=\s*["'][^"']{8,}["']/ nocase
        $pw2 = /passwd\s*=\s*["'][^"']{8,}["']/ nocase
    condition:
        any of them
}
