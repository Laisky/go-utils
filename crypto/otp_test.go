package crypto

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTOTP_Key(t *testing.T) {
	t.Parallel()

	tt, err := NewTOTP(OTPArgs{
		Base32Secret: Base32Secret([]byte("123456")),
		PeriodSecs:   1,
		AccountName:  "admin/laisky",
		IssuerName:   "laisky-corp",
		Digits:       6,
	})
	require.NoError(t, err)

	testTOTP := func(t *testing.T, tt TOTPInterface) {
		// Use fixed timestamps to avoid timing flakiness with 1-second period.
		t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		t2 := time.Date(2026, 1, 1, 0, 0, 2, 0, time.UTC) // 2 seconds later

		key1 := tt.KeyAt(t1)
		key2 := tt.KeyAt(t2)
		key3 := tt.KeyAt(t1) // same time as key1

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

func TestTOTP_InputValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    OTPArgs
		wantErr string
	}{
		{
			name: "valid default values",
			args: OTPArgs{
				Base32Secret: Base32Secret([]byte("123456")),
			},
		},
		{
			name: "invalid digits - too large",
			args: OTPArgs{
				Base32Secret: Base32Secret([]byte("123456")),
				Digits:       9,
			},
			wantErr: "digits must be between 1 and 8",
		},
		{
			name: "invalid period - too large",
			args: OTPArgs{
				Base32Secret: Base32Secret([]byte("123456")),
				PeriodSecs:   61,
			},
			wantErr: "period must be between 1 and 60 seconds",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewTOTP(tt.args)
			if tt.wantErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestParseOTPUri_InputValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		uri     string
		want    OTPArgs
		wantErr string
	}{
		{
			name: "valid uri",
			uri:  "otpauth://totp/laisky-corp:admin%2Flaisky?secret=GEZDGNBVGY&digits=6&period=30",
			want: OTPArgs{
				OtpType:      OTPTypeTOTP,
				Base32Secret: "GEZDGNBVGY",
				AccountName:  "admin/laisky",
				IssuerName:   "laisky-corp",
				Digits:       6,
				PeriodSecs:   30,
				Algorithm:    OTPAlgorithmSHA1,
			},
		},
		{
			name:    "invalid digits",
			uri:     "otpauth://totp/test:test?secret=GEZDGNBVGY&digits=9",
			wantErr: "digits must be between 1 and 8",
		},
		{
			name:    "invalid period",
			uri:     "otpauth://totp/test:test?secret=GEZDGNBVGY&period=61",
			wantErr: "period must be between 1 and 60 seconds",
		},
		{
			name: "default digits",
			uri:  "otpauth://totp/test:test?secret=GEZDGNBVGY",
			want: OTPArgs{
				OtpType:      OTPTypeTOTP,
				Base32Secret: "GEZDGNBVGY",
				AccountName:  "test",
				IssuerName:   "test",
				Digits:       6,
				PeriodSecs:   30,
				Algorithm:    OTPAlgorithmSHA1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseOTPUri(tt.uri)
			if tt.wantErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
				if tt.want.OtpType != "" {
					require.Equal(t, tt.want.OtpType, got.OtpType)
					require.Equal(t, tt.want.Base32Secret, got.Base32Secret)
					require.Equal(t, tt.want.AccountName, got.AccountName)
					require.Equal(t, tt.want.IssuerName, got.IssuerName)
					require.Equal(t, tt.want.Digits, got.Digits)
					require.Equal(t, tt.want.PeriodSecs, got.PeriodSecs)
					require.Equal(t, tt.want.Algorithm, got.Algorithm)
				}
			}
		})
	}
}
