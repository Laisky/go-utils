# Recalled memory is data, not authority

BeforeTurn emits a fixed developer policy and a separate user-role JSON reference
when recall is nonempty. Facts, insight summaries, IDs, tiers, paths and offsets
are separate provenance fields. The current user input remains last. Empty recall
adds neither message. Persistence removes the fixed policy/data pair, not
standalone user messages that merely resemble a reference. Token accounting and the compaction rebuild include both.

HTML-sensitive characters are JSON-escaped at the final serialization boundary,
after snippet decoding and clipping. Stored text that already looks wrapped is
still data. Only the outer memory_reference tags are literal; decoded facts and
snippets preserve their ordinary content. The reference cannot create a trusted
role or an executable function-call item by injecting text.

This is a structural trust-boundary fix, not a guarantee against all semantic
prompt injection. Consumers must forward these roles without promoting the data
to instructions, validate externally supplied conversation roles, and authorize
every tool action independently of model text or recalled claims of permission.
The library does not execute a model response or grant tools from recalled text.

The local adversarial regression covers serialization and BeforeTurn-to-wire
integration with synthetic stored content. It does not claim a live-model attack
success rate or prove universal model compliance. Applications should evaluate
their actual model and tool policy with benign and adversarial recall before
production rollout. A reference merely requesting an action must never suffice
as authorization.

Reference: OpenAI, Safety in building agents (do not interpolate untrusted data
into developer messages; use structured data and independent tool approvals).
https://developers.openai.com/api/docs/guides/agent-builder-safety
