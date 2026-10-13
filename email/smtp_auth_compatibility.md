# TLS SMTP authentication compatibility

The TLS sender negotiates mechanisms from the post-TLS EHLO capability set.
It retains gomail's preference: CRAM-MD5 when advertised, otherwise PLAIN,
otherwise LOGIN. Mechanisms are exact tokens, not substring matches. This
restores valid LOGIN-only and CRAM-MD5-only relays without relaxing TLS,
certificate validation, host verification or transport deadlines.

CRAM-MD5 is retained for interoperability, not selected as a modern password
storage algorithm. LOGIN and PLAIN credentials are only transmitted inside TLS.
The client does not retry a different mechanism after authentication failure.
Unknown-only AUTH capabilities fail before sending credentials or mail data.

Existing explicit plaintext opt-out/custom-dialer behavior is unchanged. No new
API or migration is necessary for this wire-compatible negotiation repair.
The loopback tests actually challenge and verify disposable LOGIN/CRAM-MD5
credentials; they do not accept unadvertised PLAIN as a successful control.
