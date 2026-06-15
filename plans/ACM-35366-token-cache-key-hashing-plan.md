# ACM-35366: Implementation Plan — Hash Token Cache Keys with SHA-256

**Jira:** https://redhat.atlassian.net/browse/ACM-35367
**Parent:** https://redhat.atlassian.net/browse/ACM-35366
**Date:** 2026-06-12
**Reference:** search-mcp-server fix — https://github.com/stolostron/search-mcp-server/pull/59

---

## Problem

`pkg/rbac/tokenReview.go` caches validated token reviews using the raw bearer token
string as the map key in `Cache.tokenReviews`:

```go
// GetTokenReview — line 39 (read)
cachedTR, tokenExists := c.tokenReviews[token]

// GetTokenReview — line 48 (write)
c.tokenReviews[token] = cachedTR
```

The map comment in `cache.go` line 19 even documents this: `//Key:ClientToken`.
Raw tokens — which can carry cluster-admin privileges — are retained in heap memory
for the duration of `AuthCacheTTL`. Any memory exposure path (core dump, heap
read, debug log printing the map) yields high-privilege credentials verbatim.

Note: `tokenReviewCache.token` (line 43) is the raw token sent to the Kubernetes
TokenReview API. That field must remain unchanged — only the map key changes.

---

## Fix

Replace the raw token string used as the map key with its SHA-256 hex digest.
SHA-256 is deterministic, so cache hit/miss semantics are unchanged.

---

## Changes

### 1. `pkg/rbac/tokenReview.go`

**Add import** `"crypto/sha256"` and `"fmt"` (check if `fmt` is already imported).

**Add helper** (above `GetTokenReview`):

```go
// hashToken returns the SHA-256 hex digest of a raw token value.
// Used as the cache key to avoid retaining bearer tokens in heap memory.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return fmt.Sprintf("%x", sum)
}
```

**Update read path** (line 39):

```go
// before
cachedTR, tokenExists := c.tokenReviews[token]

// after
cachedTR, tokenExists := c.tokenReviews[hashToken(token)]
```

**Update write path** (line 48):

```go
// before
c.tokenReviews[token] = cachedTR

// after
c.tokenReviews[hashToken(token)] = cachedTR
```

`tokenReviewCache.token` (line 43) stays as `token: token` — no change.

---

### 2. `pkg/rbac/tokenReview_test.go` — update existing tests

Two existing tests directly insert raw token strings as map keys. After the fix,
`GetTokenReview` will look up the hashed key and miss them. Both need updating.

**`Test_IsValidToken_usingCache`** (line 45):

```go
// before
mock_cache.tokenReviews["1234567890"] = &tokenReviewCache{ ... }
result, err := mock_cache.IsValidToken(context.TODO(), "1234567890")

// after — seed with the hashed key so the lookup matches
mock_cache.tokenReviews[hashToken("1234567890")] = &tokenReviewCache{ ... }
result, err := mock_cache.IsValidToken(context.TODO(), "1234567890")
```

**`Test_IsValidToken_expiredCache`** (lines 70, 82, 92):

```go
// before
mock_cache.tokenReviews["1234567890-expired"] = &tokenReviewCache{ ... }
result, err := mock_cache.IsValidToken(context.TODO(), "1234567890-expired")
if mock_cache.tokenReviews["1234567890-expired"].meta.updatedAt...

// after
mock_cache.tokenReviews[hashToken("1234567890-expired")] = &tokenReviewCache{ ... }
result, err := mock_cache.IsValidToken(context.TODO(), "1234567890-expired")
if mock_cache.tokenReviews[hashToken("1234567890-expired")].meta.updatedAt...
```

---

### 3. `pkg/rbac/tokenReview_test.go` — add new tests

```go
// Test_hashToken_keyIsHashed asserts that GetTokenReview stores entries under the
// SHA-256 hash of the token, not the raw token string.
func Test_hashToken_keyIsHashed(t *testing.T) {
    mock_cache := newMockCache()
    token := "super-secret-bearer-token"

    // Trigger a (failed) TokenReview — this seeds the cache entry.
    mock_cache.GetTokenReview(context.TODO(), token)

    _, rawPresent := mock_cache.tokenReviews[token]
    if rawPresent {
        t.Error("raw token must not be stored as a cache key")
    }

    _, hashedPresent := mock_cache.tokenReviews[hashToken(token)]
    if !hashedPresent {
        t.Error("SHA-256 hash of token must be used as the cache key")
    }
}
```

---

## Acceptance Criteria

- [ ] `hashToken` helper added; `crypto/sha256` imported
- [ ] `GetTokenReview` read (line 39) and write (line 48) use `hashToken(token)` as the map key
- [ ] `tokenReviewCache.token` (Kubernetes API payload) unchanged
- [ ] `Test_IsValidToken_usingCache` updated to seed the hashed key
- [ ] `Test_IsValidToken_expiredCache` updated to seed and assert via hashed key
- [ ] `Test_hashToken_keyIsHashed` added and passes
- [ ] `make test` passes
- [ ] `make test-race` passes
- [ ] `make lint` reports no new issues

---

## Out of Scope

- WebSocket `AuthToken` field in `pkg/server/websocket.go` (tracked separately)
- Changing `AuthCacheTTL` or eviction policy
- Encrypting cached `TokenReview` values
