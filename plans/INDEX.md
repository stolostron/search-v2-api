sessions:
---
- date: "2026-06-12"
  title: "[search-v2-api] Implement SHA-256 token cache key hashing"
  jira: "ACM-35367"
  jira_url: "https://redhat.atlassian.net/browse/ACM-35367"
  pr: ~
  plan: "plans/ACM-35366-token-cache-key-hashing-plan.md"
  summary: "Replace raw bearer tokens as tokenReviews map keys with SHA-256 hashes in pkg/rbac/tokenReview.go to eliminate plaintext credential exposure in heap memory"
