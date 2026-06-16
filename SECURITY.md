# Security Policy

## Supported versions

This is a project template. Security fixes land on `main` and the newest tag.
Track `TEMPLATE_VERSION` and pull template-owned paths on update (see
`UPGRADING.md`). Older tags are not separately patched.

## Reporting a vulnerability

Report privately — do not open a public issue for an undisclosed vulnerability.

- Preferred: GitHub **Security advisories** → *Report a vulnerability* (enable
  "Private vulnerability reporting" in repo settings).
- Otherwise: email the maintainers listed in the repository metadata.

Please include affected version/commit, reproduction steps, and impact. Expect an
acknowledgement within a few business days.

## Security model (what the template gives you)

The defenses live in `internal/core/` and are exercised by tests:

- **Strict CSP** with a per-request nonce; no `unsafe-inline` (only the nonce'd
  import map is inline). `internal/core/httpx/headers.go`.
- **CSRF**: masked double-submit cookie, constant-time compare, `__Host-` prefix
  in production. Verified on every unsafe method. `internal/core/httpx/csrf.go`.
- **Proxy-aware real client IP**: client-IP headers are trusted only from
  `TRUSTED_PROXIES`. `internal/core/httpx/realip.go`.
- **Rate limiting** (defense-in-depth behind the CF edge), **request-size caps**,
  **panic recovery** (no stack leak), and a **request ID** on every response/log
  line. `internal/core/httpx/`.
- **No SQL injection**: every query is parameterized via sqlc.
- **Hardened runtime**: distroless, nonroot, read-only rootfs, dropped caps; the
  image build is offline/hermetic from a committed vendor tree.
- **Supply chain**: SHA-pinned Actions, govulncheck, Trivy (fs + image), CodeQL,
  gosec (via golangci-lint), a Syft SBOM, and keyless cosign signing +
  SBOM attestation on release.

### Threat-model notes / your responsibility

- App-level rate limiting is **single-instance** and a second layer; the primary
  limiter is your edge (e.g. Cloudflare).
- `pprof` binds to an internal address and is **off by default** — keep it off in
  production or restrict network access to it.
- Set `APP_ENV=production` so cookies get `Secure` + `__Host-` and HSTS is sent.
- Configure `TRUSTED_PROXIES` to exactly your proxy ranges, never `0.0.0.0/0`.

## Adding authentication

The optional `internal/feature/auth/` slice is a starting point: email/password
registration and login with **server-side (DB) sessions**, a `RequireAuth`
wrapper, and **PBKDF2-HMAC-SHA256** password hashing (stdlib only, so the
template keeps its two-runtime-dependency footprint).

If you need stronger password hashing, swap `hashPassword`/`verifyPassword` in
`internal/feature/auth/password.go` for **argon2id**
(`golang.org/x/crypto/argon2`) — a deliberate, well-justified runtime dependency
to add in your fork. When you do:

- keep the encoded-hash format self-describing (algorithm + params per row) so you
  can re-hash on the next login without a migration;
- preserve the constant-time comparison and the uniform "invalid email or
  password" response (no user enumeration);
- keep sessions server-side if you need revocation ("log out everywhere"), and
  rotate the session token on login to prevent fixation.
