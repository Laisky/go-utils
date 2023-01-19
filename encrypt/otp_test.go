package encrypt

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTOTP_Key(t *testing.T) {
	tt, err := NewTOTP(OTPArgs{
		Base32Secret: Base32Secret([]byte("123456")),
		PeriodSecs:   1,
		AccountName:  "admin/laisky",
		IssuerName:   "laisky-corp",
		Digits:       6,
	})
	require.NoError(t, err)

	testTOTP := func(t *testing.T, tt TOTPInterface) {
		key1 := tt.Key()
		time.Sleep(2 * time.Second)
		key2 := tt.Key()
		key3 := tt.KeyAt(time.Now().Add(-2 * time.Second))

		require.Len(t, key1, 6)
		require.Len(t, key2, 6)
		require.Equal(t, key1, key3)
		require.NotEqual(t, key1, key2)

		require.Equal(t, "otpauth://totp/laisky-corp:admin%252Flaisky?issuer=laisky-corp&period=1&secret=GEZDGNBVGY", tt.URI())
	}

	testTOTP(t, tt)

	arg, err := ParseOTPUri(tt.URI())
	require.Equal(t, OTPTypeTOTP, arg.OtpType)
	require.Equal(t, Base32Secret([]byte("123456")), arg.Base32Secret)
	require.Equal(t, "admin/laisky", arg.AccountName)
	require.Equal(t, "laisky-corp", arg.IssuerName)
	require.Equal(t, "sha1", string(arg.Algorithm))
	require.Equal(t, 0, arg.InitialCount)
	require.Equal(t, uint(6), arg.Digits)
	require.Equal(t, uint(1), arg.PeriodSecs)

	require.NoError(t, err)
	tt, err = NewTOTP(arg)
	require.NoError(t, err)
	testTOTP(t, tt)
}
