// package Threshold cryptosystem
//
// Threshold cryptography uses multiple keys or parties to
// generate secure cryptographic keys/signatures.
//
// The secret key is split into shares for distribution.
// Multiple parties must combine their shares to reconstruct the original key,
// preventing a single party from accessing it alone.
//
// Key generation can involve combining independent key pairs for added security.
// This approach reduces the risk of a single party compromising the private key and the system.
//
//   - https://en.wikipedia.org/wiki/Threshold_cryptosystem
//   - [Practical Threshold Signatures.pdf](https://1drv.ms/b/s!Au45o0W1gVVLva5GL6spMZOeKyOU7A?e=OnAcGG)
package threshold
