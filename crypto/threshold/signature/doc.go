// Package signature provides an implementation of threshold signatures.
//
// Threshold signatures use cryptography to allow a group of signers to collaboratively sign a message
// such that any subset of the signers can produce a valid signature, but no smaller group can.
//
// This package defines functions and types for creating and verifying threshold signatures,
// including a struct for specifying and enforcing a minimum threshold signature requirement.
//
// To prevent private key exposure or signature malleability,
// this package does not expose raw private keys or signatures.
package signature
