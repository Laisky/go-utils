package crypto

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"io"
	"math"
	"testing"
	"testing/iotest"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
)

// streamTestChunk is the small chunk size used to build multi-chunk test streams.
const streamTestChunk = AEADStreamMinChunkSize

// streamTestKey returns a fresh random AES key of n bytes for stream tests.
func streamTestKey(t *testing.T, n int) []byte {
	t.Helper()
	key := make([]byte, n)
	_, err := rand.Read(key)
	require.NoError(t, err)
	return key
}

// streamTestPlaintext returns n deterministic plaintext bytes.
func streamTestPlaintext(n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = byte(i*7 + 3)
	}
	return out
}

// streamEncrypt encrypts plaintext with AEADStreamEncrypt and returns the full ciphertext.
func streamEncrypt(t *testing.T, key, plaintext []byte, opts ...AEADStreamOption) []byte {
	t.Helper()
	r, err := AEADStreamEncrypt(key, bytes.NewReader(plaintext), opts...)
	require.NoError(t, err)
	ct, err := io.ReadAll(r)
	require.NoError(t, err)
	return ct
}

// streamDecrypt decrypts ciphertext and returns the plaintext read so far and
// the first error from either construction or reading.
func streamDecrypt(key, ciphertext []byte) ([]byte, error) {
	r, err := AEADStreamDecrypt(key, bytes.NewReader(ciphertext))
	if err != nil {
		return nil, err
	}
	return io.ReadAll(r)
}

// streamChunks splits a small-chunk test stream into its header and chunk slices.
func streamChunks(ct []byte) (header []byte, chunks [][]byte) {
	header, rest := ct[:aeadStreamHeaderLen], ct[aeadStreamHeaderLen:]
	full := streamTestChunk + AesGcmTagLen
	for len(rest) > full {
		chunks = append(chunks, rest[:full])
		rest = rest[full:]
	}
	return header, append(chunks, rest)
}

// streamJoin concatenates header and chunks into a new ciphertext.
func streamJoin(header []byte, chunks ...[]byte) []byte {
	out := append([]byte(nil), header...)
	for _, c := range chunks {
		out = append(out, c...)
	}
	return out
}

// TestAESStreamSecurity_CTRIsMalleableLowLevelPrimitive documents that the raw
// CTR primitive is confidentiality-only, which is why it must never be the
// migration target of the authenticated AesReaderWrapper. Regression for issue #71.
func TestAESStreamSecurity_CTRIsMalleableLowLevelPrimitive(t *testing.T) {
	t.Parallel()

	key := streamTestKey(t, 32)
	msg := []byte(`{"amount":100,"to":"alice"}`)
	enc, err := AesCtrStreamEncrypt(key, bytes.NewReader(msg))
	require.NoError(t, err)
	ct, err := io.ReadAll(enc)
	require.NoError(t, err)

	const k = 11
	ct[16+k] ^= 0x01
	dec, err := AesCtrStreamDecrypt(key, bytes.NewReader(ct))
	require.NoError(t, err)
	got, err := io.ReadAll(dec)
	require.NoError(t, err, "CTR is unauthenticated by contract")
	require.Equal(t, msg[k]^0x01, got[k])
}

// TestAESStreamSecurity_GCMControlRejectsTampering shows that the authenticated
// GCM APIs (including the deprecated AesReaderWrapper) reject the same
// tampering. Regression for issue #71.
func TestAESStreamSecurity_GCMControlRejectsTampering(t *testing.T) {
	t.Parallel()

	key := streamTestKey(t, 32)
	msg := []byte(`{"amount":100,"to":"alice"}`)
	ct, err := AEADEncrypt(key, msg, nil)
	require.NoError(t, err)
	ct[AesGcmIvLen+11] ^= 0x01

	_, err = AEADDecrypt(key, ct, nil)
	require.Error(t, err)
	_, err = NewAesReaderWrapper(bytes.NewReader(ct), key)
	require.Error(t, err)
}

// TestAESStreamSecurity_ReplacementRejectsTampering checks that the documented
// replacement for AesReaderWrapper rejects the CTR bit-flip attack.
// Regression for issue #71 (it failed when the guidance pointed at CTR).
func TestAESStreamSecurity_ReplacementRejectsTampering(t *testing.T) {
	t.Parallel()

	key := streamTestKey(t, 32)
	msg := []byte(`{"amount":100,"to":"alice"}`)
	ct := streamEncrypt(t, key, msg)
	ct[aeadStreamHeaderLen+11] ^= 0x01

	got, err := streamDecrypt(key, ct)
	require.Error(t, err)
	require.Empty(t, got, "no plaintext of an unauthenticated chunk may be released")
}

// TestAESStreamSecurity_RoundTrips covers empty, boundary and multi-chunk
// streams for all key sizes. Regression for issue #71.
func TestAESStreamSecurity_RoundTrips(t *testing.T) {
	t.Parallel()

	sizes := []int{0, 1, streamTestChunk - 1, streamTestChunk, streamTestChunk + 1,
		2 * streamTestChunk, streamTestChunk*3 + streamTestChunk/2}
	for _, keyLen := range []int{16, 24, 32} {
		key := streamTestKey(t, keyLen)
		for _, n := range sizes {
			msg := streamTestPlaintext(n)
			ct := streamEncrypt(t, key, msg, WithAEADStreamChunkSize(streamTestChunk))
			got, err := streamDecrypt(key, ct)
			require.NoError(t, err, "key %d size %d", keyLen, n)
			require.Equal(t, msg, append([]byte{}, got...), "key %d size %d", keyLen, n)
		}
	}

	// Default 64 KiB chunks with 3.5 chunks of data.
	key := streamTestKey(t, 32)
	msg := streamTestPlaintext(AEADStreamDefaultChunkSize*3 + AEADStreamDefaultChunkSize/2)
	ct := streamEncrypt(t, key, msg)
	require.Len(t, ct, aeadStreamHeaderLen+len(msg)+4*AesGcmTagLen)
	got, err := streamDecrypt(key, ct)
	require.NoError(t, err)
	require.Equal(t, msg, got)

	// Empty plaintext is exactly one empty final chunk.
	ct = streamEncrypt(t, key, nil)
	require.Len(t, ct, aeadStreamHeaderLen+AesGcmTagLen)
}

// TestAESStreamSecurity_EveryBitFlipRejected flips one bit in every header,
// ciphertext and tag byte of a multi-chunk stream. Regression for issue #71.
func TestAESStreamSecurity_EveryBitFlipRejected(t *testing.T) {
	t.Parallel()

	key := streamTestKey(t, 32)
	ct := streamEncrypt(t, key, streamTestPlaintext(streamTestChunk*2+10), WithAEADStreamChunkSize(streamTestChunk))
	for i := range ct {
		for _, bit := range []byte{0x01, 0x80} {
			tampered := append([]byte(nil), ct...)
			tampered[i] ^= bit
			_, err := streamDecrypt(key, tampered)
			require.Error(t, err, "flip %#x at offset %d accepted", bit, i)
		}
	}
}

// TestAESStreamSecurity_TruncationRejected truncates the stream at every
// length, including chunk boundaries and mid-chunk. Regression for issue #71.
func TestAESStreamSecurity_TruncationRejected(t *testing.T) {
	t.Parallel()

	key := streamTestKey(t, 16)
	for _, n := range []int{0, streamTestChunk * 2, streamTestChunk*3 + 5} {
		ct := streamEncrypt(t, key, streamTestPlaintext(n), WithAEADStreamChunkSize(streamTestChunk))
		for cut := range len(ct) {
			_, err := streamDecrypt(key, ct[:cut])
			require.Error(t, err, "plaintext %d truncated to %d accepted", n, cut)
		}
	}
}

// TestAESStreamSecurity_ChunkManipulationRejected covers reordering,
// duplication, dropped chunks, a missing final chunk and trailing data.
// Regression for issue #71.
func TestAESStreamSecurity_ChunkManipulationRejected(t *testing.T) {
	t.Parallel()

	key := streamTestKey(t, 32)
	ct := streamEncrypt(t, key, streamTestPlaintext(streamTestChunk*3+7), WithAEADStreamChunkSize(streamTestChunk))
	h, c := streamChunks(ct)
	require.Len(t, c, 4)

	cases := map[string][]byte{
		"reorder":            streamJoin(h, c[1], c[0], c[2], c[3]),
		"duplicate":          streamJoin(h, c[0], c[1], c[1], c[2], c[3]),
		"drop middle":        streamJoin(h, c[0], c[2], c[3]),
		"missing final":      streamJoin(h, c[0], c[1], c[2]),
		"final first":        streamJoin(h, c[3]),
		"trailing byte":      append(streamJoin(h, c...), 0),
		"trailing chunk":     streamJoin(h, c[0], c[1], c[2], c[3], c[0]),
		"trailing final dup": streamJoin(h, c[0], c[1], c[2], c[3], c[3]),
		"header only":        streamJoin(h),
	}
	for name, tampered := range cases {
		_, err := streamDecrypt(key, tampered)
		require.Error(t, err, name)
	}

	// A different stream's chunk (same key, different salt) is rejected.
	other := streamEncrypt(t, key, streamTestPlaintext(streamTestChunk*3+7), WithAEADStreamChunkSize(streamTestChunk))
	_, oc := streamChunks(other)
	_, err := streamDecrypt(key, streamJoin(h, c[0], oc[1], c[2], c[3]))
	require.Error(t, err)
}

// TestAESStreamSecurity_ReleasesOnlyAuthenticatedChunks checks that a tampered
// chunk releases none of its plaintext. Regression for issue #71.
func TestAESStreamSecurity_ReleasesOnlyAuthenticatedChunks(t *testing.T) {
	t.Parallel()

	key := streamTestKey(t, 32)
	msg := streamTestPlaintext(streamTestChunk*3 + 7)
	ct := streamEncrypt(t, key, msg, WithAEADStreamChunkSize(streamTestChunk))
	ct[aeadStreamHeaderLen+streamTestChunk+AesGcmTagLen+3] ^= 0x10 // inside chunk 1

	got, err := streamDecrypt(key, ct)
	require.Error(t, err)
	require.Equal(t, msg[:streamTestChunk], got, "only chunk 0 is authenticated")
}

// TestAESStreamSecurity_KeyAndHeaderValidation covers wrong keys, invalid key
// sizes and hostile header fields. Regression for issue #71.
func TestAESStreamSecurity_KeyAndHeaderValidation(t *testing.T) {
	t.Parallel()

	key := streamTestKey(t, 32)
	ct := streamEncrypt(t, key, streamTestPlaintext(100), WithAEADStreamChunkSize(streamTestChunk))

	_, err := streamDecrypt(streamTestKey(t, 32), ct)
	require.Error(t, err, "wrong key")
	_, err = AEADStreamDecrypt(key[:20], bytes.NewReader(ct))
	require.Error(t, err, "invalid key size")
	_, err = AEADStreamEncrypt(key[:15], bytes.NewReader(nil))
	require.Error(t, err, "invalid key size")
	_, err = AEADStreamEncrypt(key, nil)
	require.Error(t, err)
	_, err = AEADStreamDecrypt(key, nil)
	require.Error(t, err)

	off := len(aeadStreamMagic)
	for name, mutate := range map[string]func(h []byte){
		"magic":         func(h []byte) { h[0] ^= 0xff },
		"version":       func(h []byte) { h[off] = 2 },
		"chunk zero":    func(h []byte) { binary.BigEndian.PutUint32(h[off+1:], 0) },
		"chunk small":   func(h []byte) { binary.BigEndian.PutUint32(h[off+1:], AEADStreamMinChunkSize-1) },
		"chunk huge":    func(h []byte) { binary.BigEndian.PutUint32(h[off+1:], math.MaxUint32) },
		"chunk too big": func(h []byte) { binary.BigEndian.PutUint32(h[off+1:], AEADStreamMaxChunkSize+1) },
	} {
		tampered := append([]byte(nil), ct...)
		mutate(tampered)
		_, err := AEADStreamDecrypt(key, bytes.NewReader(tampered))
		require.Error(t, err, name)
	}

	// An in-range chunk size change is caught by authentication.
	tampered := append([]byte(nil), ct...)
	binary.BigEndian.PutUint32(tampered[off+1:], streamTestChunk*2)
	_, err = streamDecrypt(key, tampered)
	require.Error(t, err)

	for _, size := range []int{0, AEADStreamMinChunkSize - 1, AEADStreamMaxChunkSize + 1} {
		_, err = AEADStreamEncrypt(key, bytes.NewReader(nil), WithAEADStreamChunkSize(size))
		require.Error(t, err, "chunk size %d", size)
	}
	_, err = AEADStreamDecrypt(key, bytes.NewReader(ct[:aeadStreamHeaderLen-1]))
	require.Error(t, err, "short header")
}

// TestAESStreamSecurity_FreshSaltPerStream checks that equal inputs produce
// unrelated ciphertexts. Regression for issue #71.
func TestAESStreamSecurity_FreshSaltPerStream(t *testing.T) {
	t.Parallel()

	key := streamTestKey(t, 32)
	msg := streamTestPlaintext(10)
	a, b := streamEncrypt(t, key, msg), streamEncrypt(t, key, msg)
	require.NotEqual(t, a[aeadStreamHeaderLen-aeadStreamSaltLen:aeadStreamHeaderLen],
		b[aeadStreamHeaderLen-aeadStreamSaltLen:aeadStreamHeaderLen])
	require.NotEqual(t, a[aeadStreamHeaderLen:], b[aeadStreamHeaderLen:])
}

// TestAESStreamSecurity_PartialReads exercises tiny and irregular reads on both
// sides of the stream. Regression for issue #71.
func TestAESStreamSecurity_PartialReads(t *testing.T) {
	t.Parallel()

	key := streamTestKey(t, 32)
	msg := streamTestPlaintext(streamTestChunk*3 + streamTestChunk/2)
	wrappers := map[string]func(io.Reader) io.Reader{
		"one byte": iotest.OneByteReader,
		"half":     iotest.HalfReader,
		"data err": iotest.DataErrReader,
	}
	for name, wrap := range wrappers {
		enc, err := AEADStreamEncrypt(key, wrap(bytes.NewReader(msg)), WithAEADStreamChunkSize(streamTestChunk))
		require.NoError(t, err, name)
		ct, err := io.ReadAll(wrap(enc))
		require.NoError(t, err, name)

		dec, err := AEADStreamDecrypt(key, wrap(bytes.NewReader(ct)))
		require.NoError(t, err, name)
		got, err := io.ReadAll(wrap(dec))
		require.NoError(t, err, name)
		require.Equal(t, msg, got, name)

		dec, err = AEADStreamDecrypt(key, wrap(bytes.NewReader(ct)))
		require.NoError(t, err, name)
		require.NoError(t, iotest.TestReader(dec, msg), name)
	}

	enc, err := AEADStreamEncrypt(key, bytes.NewReader(msg))
	require.NoError(t, err)
	n, err := enc.Read(nil)
	require.NoError(t, err)
	require.Zero(t, n)
}

// TestAESStreamSecurity_IOErrorPropagation checks that source errors surface on
// both sides and that a stream cut by an error is not accepted. Regression for
// issue #71.
func TestAESStreamSecurity_IOErrorPropagation(t *testing.T) {
	t.Parallel()

	key := streamTestKey(t, 32)
	errBoom := errors.New("boom")
	msg := streamTestPlaintext(streamTestChunk * 3)

	src := io.MultiReader(bytes.NewReader(msg[:streamTestChunk+5]), iotest.ErrReader(errBoom))
	enc, err := AEADStreamEncrypt(key, src, WithAEADStreamChunkSize(streamTestChunk))
	require.NoError(t, err)
	partial, err := io.ReadAll(enc)
	require.ErrorIs(t, err, errBoom)
	_, err = enc.Read(make([]byte, 8))
	require.ErrorIs(t, err, errBoom, "error is sticky")
	_, err = streamDecrypt(key, partial)
	require.Error(t, err, "stream cut by an I/O error has no final chunk")

	ct := streamEncrypt(t, key, msg, WithAEADStreamChunkSize(streamTestChunk))
	dec, err := AEADStreamDecrypt(key,
		io.MultiReader(bytes.NewReader(ct[:aeadStreamHeaderLen+100]), iotest.ErrReader(errBoom)))
	require.NoError(t, err)
	_, err = io.ReadAll(dec)
	require.ErrorIs(t, err, errBoom)

	_, err = AEADStreamDecrypt(key, iotest.ErrReader(errBoom))
	require.ErrorIs(t, err, errBoom)
}

// TestAESStreamSecurity_NonCanonicalAndCounterLimits covers an empty final
// chunk after data and chunk-counter exhaustion. Regression for issue #71.
func TestAESStreamSecurity_NonCanonicalAndCounterLimits(t *testing.T) {
	t.Parallel()

	key := streamTestKey(t, 32)
	ct := streamEncrypt(t, key, streamTestPlaintext(streamTestChunk), WithAEADStreamChunkSize(streamTestChunk))
	header := ct[:aeadStreamHeaderLen]
	aead, err := newAEADStreamCipher(key, header)
	require.NoError(t, err)

	var nonce [12]byte
	aeadStreamNonce(nonce[:], 0, false)
	c0 := aead.Seal(nil, nonce[:], streamTestPlaintext(streamTestChunk), nil)
	aeadStreamNonce(nonce[:], 1, true)
	c1 := aead.Seal(nil, nonce[:], nil, nil)
	_, err = streamDecrypt(key, streamJoin(header, c0, c1))
	require.ErrorContains(t, err, "non-canonical")

	dec, err := AEADStreamDecrypt(key, bytes.NewReader(ct))
	require.NoError(t, err)
	dec.(*aeadStreamDecrypter).counter = math.MaxUint64
	_, err = io.ReadAll(dec)
	require.ErrorContains(t, err, "exhausted")

	enc, err := AEADStreamEncrypt(key, bytes.NewReader(nil))
	require.NoError(t, err)
	enc.(*aeadStreamEncrypter).counter = math.MaxUint64
	_, err = io.ReadAll(enc)
	require.ErrorContains(t, err, "exhausted")
}
