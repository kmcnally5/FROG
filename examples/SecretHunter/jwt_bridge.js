// jwt_bridge.js — pure-Node JWT decoder for SecretHunter.
//
// No npm dependencies on purpose: a JWT is just base64url-encoded JSON for
// header + payload + signature, and decoding doesn't need a library. The
// jsonwebtoken package adds verification, which we don't do here — we never
// have the signing key, and unverified-but-decoded is exactly what a static
// scanner needs to surface risk.
//
// What we report on each token (decode_batch):
//   alg            — algorithm declared in the header (none/HS256/RS256/...)
//   typ            — token type (usually "JWT")
//   alg_warning    — "alg:none" or "weak_alg" (HS256 with a short shared secret
//                    is widely brute-forceable) when applicable
//   iss/sub/aud    — issuer / subject / audience claims, if present
//   exp/iat/nbf    — timestamps as ISO strings (claims are seconds since epoch)
//   expired        — bool, exp < now
//   expires_soon   — bool, exp within the next 7 days
//   scopes         — array of strings parsed from "scope" or "scopes" claim
//   error          — non-null when decoding failed (malformed base64, bad JSON)
//
// All flagged at the SecretHunter side regardless of validity — a leaked JWT
// is a leak even if it's already expired (the underlying secret used to sign
// it may still be live).

'use strict';

const {handler, serve} = require('klex_bridge');


// Algorithms we consider weak by today's standards. HS256 itself isn't broken,
// but the wild has so many low-entropy shared secrets that flagging HS-family
// gives the user a useful pointer to rotate. RS-family and ES-family use real
// asymmetric crypto and don't get this flag.
const WEAK_ALGS = new Set(['HS1', 'HS256']);


// base64url → utf8 string. Pads, swaps url-safe chars, decodes.
function b64urlDecode(s) {
    if (typeof s !== 'string' || s.length === 0) return null;
    const pad = s.length % 4 === 0 ? '' : '='.repeat(4 - (s.length % 4));
    const std = s.replace(/-/g, '+').replace(/_/g, '/') + pad;
    try {
        return Buffer.from(std, 'base64').toString('utf8');
    } catch (_) {
        return null;
    }
}


function decodeOne(token) {
    if (typeof token !== 'string' || token.length === 0) {
        return {error: 'empty token'};
    }
    const parts = token.split('.');
    if (parts.length !== 3) {
        return {error: `wrong segment count (got ${parts.length}, expected 3)`};
    }

    const headerJson  = b64urlDecode(parts[0]);
    const payloadJson = b64urlDecode(parts[1]);
    if (!headerJson || !payloadJson) {
        return {error: 'base64url decode failed'};
    }

    let header, payload;
    try { header  = JSON.parse(headerJson);  } catch (_) { return {error: 'header JSON malformed'};  }
    try { payload = JSON.parse(payloadJson); } catch (_) { return {error: 'payload JSON malformed'}; }

    const alg = (header && typeof header.alg === 'string') ? header.alg : 'unknown';
    const typ = (header && typeof header.typ === 'string') ? header.typ : '';

    // Algorithm flags. "none" is a genuine spec footgun — accepting a token
    // with alg:none means anyone can forge any claim — so it's its own bucket.
    let algWarning = null;
    if (alg === 'none' || alg === 'None' || alg === 'NONE') {
        algWarning = 'alg:none — token claims are unauthenticated; any holder can forge';
    } else if (WEAK_ALGS.has(alg)) {
        algWarning = `${alg} relies on a shared secret — brute-forceable when the secret is short`;
    }

    // Timestamp claims. JWT spec says these are seconds since epoch. Defensive
    // about types since real-world tokens sometimes use ms or stringly-typed.
    const now    = Math.floor(Date.now() / 1000);
    const exp    = numClaim(payload, 'exp');
    const iat    = numClaim(payload, 'iat');
    const nbf    = numClaim(payload, 'nbf');
    const expired      = exp !== null && exp < now;
    const expiresSoon  = exp !== null && !expired && (exp - now) < 7 * 86400;

    // Scope parsing. OAuth-style uses a space-separated string under "scope";
    // some implementations use an array under "scopes". Accept either.
    let scopes = [];
    if (typeof payload.scope === 'string') {
        scopes = payload.scope.split(/\s+/).filter(Boolean);
    } else if (Array.isArray(payload.scopes)) {
        scopes = payload.scopes.filter((s) => typeof s === 'string');
    }

    return {
        alg,
        typ,
        alg_warning:   algWarning,
        iss:           strClaim(payload, 'iss'),
        sub:           strClaim(payload, 'sub'),
        aud:           audClaim(payload),
        exp_iso:       exp !== null ? new Date(exp * 1000).toISOString() : '',
        iat_iso:       iat !== null ? new Date(iat * 1000).toISOString() : '',
        nbf_iso:       nbf !== null ? new Date(nbf * 1000).toISOString() : '',
        expired,
        expires_soon:  expiresSoon,
        missing_exp:   exp === null,
        scopes,
        error:         null,
    };
}


// numClaim returns a number claim or null when missing / not numeric. Accepts
// stringly-typed numbers because some implementations emit them.
function numClaim(payload, key) {
    if (!payload || !(key in payload)) return null;
    const v = payload[key];
    if (typeof v === 'number' && Number.isFinite(v)) return v;
    if (typeof v === 'string' && /^\d+$/.test(v))    return Number(v);
    return null;
}

function strClaim(payload, key) {
    if (!payload) return '';
    const v = payload[key];
    return typeof v === 'string' ? v : '';
}

// "aud" can be a string or an array of strings per spec. Normalise to a
// comma-joined string for UI rendering.
function audClaim(payload) {
    if (!payload) return '';
    const v = payload.aud;
    if (typeof v === 'string') return v;
    if (Array.isArray(v))      return v.filter((s) => typeof s === 'string').join(', ');
    return '';
}


// decode_batch is the fast path SecretHunter calls — one wire trip for all
// JWT-shaped findings in a scan. Per-token failures land in the error field;
// the caller sees a structured result for every input regardless.
handler({args: [['tokens', 'array']], returns: 'array'},
    function decode_batch(tokens) {
        return tokens.map((t) => decodeOne(t));
    }
);


// decode for one-off use (tests / REPL / future per-finding rescans).
handler({args: [['token', 'string']], returns: 'hash'},
    function decode(token) {
        return decodeOne(token);
    }
);


// Health probe — used by the SecretHunter status panel to confirm the JWT
// bridge is reachable before showing the "Decode JWTs" toggle.
handler({args: [], returns: 'string'},
    function ping() { return 'pong'; }
);


serve();
