---
type: Business Rule
status: stable
verified:
  - by: "human:ArthurC"
    at: "2026-11-07T15:07:58.144675Z"
sources:
  - resource: "db/migrations/0150_reflection_candidates.sql#L1-12"
    excerpt_sha256: "sha256:eee93e69affd89b58bc515bb09df2875b22506df96bbd45ba2cd8a0d2cf8724b"
  - resource: "apps/platform/internal/trial/improvement/reflection/reflection.go#L78-104"
    excerpt_sha256: "sha256:9408b4033d02272149fdccdfacb9c37a4e8384c66ec291e7208144f2a063e835"
  - resource: "apps/platform/internal/trial/improvement/reflection/reflection.go#L302-326"
    excerpt_sha256: "sha256:9da7d5f3ae841315a5e79427c61f86dc1bae453c802971d61f4dbd2b215c1e0a"
  - resource: "apps/platform/internal/trial/improvement/reflection/round_test.go#L17-53"
    excerpt_sha256: "sha256:d02e53ea7388f59ea9c3636f6524152989ceed6425abd0c89e4f24056b1159df"
  - resource: "apps/platform/internal/trial/improvement/reflection/reflection.go#L328-357"
    excerpt_sha256: "sha256:492074fbbd20a25c354a392e5ccb3ab789eeb5d8a4eafe8dde241df1ddf49c3f"
  - resource: "apps/platform/internal/trial/improvement/reflection/round_test.go#L87-117"
    excerpt_sha256: "sha256:039df8bf1d873fe3f82a4dacdc40e35e49412264399656e4e473cf8bc9d7dc52"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/reflection_integration_test.go#L368-384"
    excerpt_sha256: "sha256:e9f743c971e08aac0206b46204cdcf8e8215aa5689e83a5e3c65ad7dd82b031d"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/reflection_integration_test.go#L496-529"
    excerpt_sha256: "sha256:5062c6d5d53b73e69257c875ef208fb3cd0d7adb271b18ad27582650e3b32795"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/reflection_integration_test.go#L531-549"
    excerpt_sha256: "sha256:e95295bdc83fe5d143160439bcaa2c26e9b453486d4d536d810d76e5b481b58d"
  - resource: "apps/platform/internal/entrypoint/api/apiserver/reflection_integration_test.go#L628-651"
    excerpt_sha256: "sha256:2b9bf7f74f3beb3990a78e9cf30d15553775cbe5176e0d141bbf9acf4ef856e8"
---
An improvement round over an evaluation batch is a model call the user starts, so it is open only to invited accounts and is admitted once, before its first model call: it is refused when model calls are halted, with 409 when it asks for more than one candidate while the operator has set no round credit cap, and with 422 when its estimate, the batch_reflection start threshold for one call times its candidates, exceeds a set round credit cap or the owner's balance. Its model calls are one debit of the usage the gateway reported, summed over the calls that answered, which lists each call's cost when every answered call of two or more reported it; a round in which no call answered is not charged.

# Relations
- Contexts: [eval](/contexts/eval)

# Examples
- With no batch_reflection statistics a balance of 69 credits is refused with 422, the model is not called and nothing is charged.
- At a threshold of 70, two candidates fit a cap of exactly 140 and are refused at a cap of 139 or a balance of 139.
- Two candidates with no cap set are refused with 409 naming reflection_round_credit_cap, before any model call.
- Three candidates whose calls report 20 prompt tokens each are one debit of 60 prompt tokens.
- Three answered calls each reporting 0.002 USD are one debit of 6000 micros listing 2000, 2000 and 2000.
