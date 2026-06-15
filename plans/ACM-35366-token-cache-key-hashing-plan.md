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
// GetTokenReview — read
cachedTR, tokenExists := c.tokenReviews[token]

// GetTokenReview — write
c.tokenReviews[token] = cachedTR
```

Raw tokens — which can carry cluster-admin privileges — are retained in heap memory
for the duration of `AuthCacheTTL`. Any memory exposure path (core dump, heap
read, debug log printing the map) yields high-privilege credentials verbatim.

Additionally, the `tokenReviewCache` struct stored the raw token as a field
(`token string`), which also kept the credential in heap memory for the lifetime
of the cached entry.

---

## Fix

1. Replace the raw token string used as the map key with its SHA-256 hex digest.
   SHA-256 is deterministic, so cache hit/miss semantics are unchanged.
2. Remove `tokenReviewCache.token` field. Pass the raw token transiently as a
   parameter to `getTokenReview(token string)` so it is never stored on the struct.

---

## Changes

### 1. `pkg/rbac/tokenReview.go`

**Add imports** `"crypto/sha256"` and `"fmt"`.

**Add helper** (above `GetTokenReview`):

```go
// hashToken returns the SHA-256 hex digest of a raw token value.
// Used as the cache key to avoid retaining bearer tokens in heap memory.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return fmt.Sprintf("%x", sum)
}
```

**Update `GetTokenReview`** — use `hashToken(token)` as the map key; remove `token` from
struct initialization:

```go
cachedTR, tokenExists := c.tokenReviews[hashToken(token)]
if !tokenExists {
	cachedTR = &tokenReviewCache{
		authClient: c.getAuthClient(),
		// no token field — passed transiently to getTokenReview
	}
	c.tokenReviews[hashToken(token)] = cachedTR
}
return cachedTR.getTokenReview(token)
```

**Remove `token string` from `tokenReviewCache` struct** and update `getTokenReview`
signature to accept it as a parameter:

```go
func (trc *tokenReviewCache) getTokenReview(token string) (*authv1.TokenReview, error) {
	// ...
	tr := authv1.TokenReview{
		Spec: authv1.TokenReviewSpec{Token: token},
	}
	// ...
}
```

---

### 2. `pkg/rbac/userData.go`

Line 150 contained a direct raw-key map access discovered during testing:

```go
// before
cache.tokenReviews[clientToken].tokenReview.Status.User.Username

// after
cache.tokenReviews[hashToken(clientToken)].tokenReview.Status.User.Username
```

---

### 3. `pkg/rbac/tokenReview_test.go` — update existing tests

Two existing tests directly inserted raw token strings as map keys. Both needed
updating to seed with hashed keys. The `token:` field in the struct literal for
`Test_IsValidToken_expiredCache` was also removed.

**`Test_IsValidToken_usingCache`**:

```go
mock_cache.tokenReviews[hashToken("1234567890")] = &tokenReviewCache{ ... }
```

**`Test_IsValidToken_expiredCache`**:

```go
mock_cache.tokenReviews[hashToken("1234567890-expired")] = &tokenReviewCache{
	// no token field
	authClient: fake.NewSimpleClientset().AuthenticationV1(),
	meta:       cacheMetadata{updatedAt: time.Now().Add(time.Duration(-5) * time.Minute)},
	// ...
}
```

---

### 4. `pkg/rbac/tokenReview_test.go` — add new test

```go
func Test_hashToken_keyIsHashed(t *testing.T) {
	mock_cache := newMockCache()
	token := "super-secret-bearer-token"

	_, err := mock_cache.GetTokenReview(context.TODO(), token)
	if err != nil {
		t.Fatalf("unexpected error from GetTokenReview: %v", err)
	}

	if _, rawPresent := mock_cache.tokenReviews[token]; rawPresent {
		t.Error("raw token must not be stored as a cache key")
	}
	if _, hashedPresent := mock_cache.tokenReviews[hashToken(token)]; !hashedPresent {
		t.Error("SHA-256 hash of token must be used as the cache key")
	}
}
```

---

### 5. `pkg/rbac/userData_test.go` and `pkg/rbac/watchCache_test.go`

All test helpers that seeded `tokenReviews` directly with raw keys were updated
to use `hashToken(...)`:

- `userData_test.go`: `setupToken` — `cache.tokenReviews[hashToken("123456")]`
- `watchCache_test.go`: `setupWatchToken` and two inline setups for
  `"watch-token-extras"` and `"watch-token-desired-extras"`

---

## Acceptance Criteria

- [x] `hashToken` helper added; `crypto/sha256` imported
- [x] `GetTokenReview` read and write paths use `hashToken(token)` as the map key
- [x] `tokenReviewCache.token` field removed; token passed transiently as parameter to `getTokenReview`
- [x] `userData.go` raw-key access updated to `hashToken(clientToken)`
- [x] `Test_IsValidToken_usingCache` updated to seed the hashed key
- [x] `Test_IsValidToken_expiredCache` updated to seed and assert via hashed key; `token:` field removed from struct literal
- [x] `Test_hashToken_keyIsHashed` added with error check and passes
- [x] `userData_test.go` and `watchCache_test.go` helpers updated to use `hashToken`
- [x] `make test` passes
- [x] `make test-race` passes
- [x] `make lint` reports no new issues

---

## Out of Scope

- WebSocket `AuthToken` field in `pkg/server/websocket.go` (tracked separately)
- Changing `AuthCacheTTL` or eviction policy
- Encrypting cached `TokenReview` values
