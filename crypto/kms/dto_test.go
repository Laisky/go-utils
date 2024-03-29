package kms

import (
	"testing"

	"github.com/stretchr/testify/require"

	gutils "github.com/Laisky/go-utils/v4"
)

func TestEncryptedItem_Unmarshal(t *testing.T) {
	t.Parallel()

	type args struct {
		KekID      uint16
		DekID      []byte
		Ciphertext []byte
	}
	tests := []struct {
		name string
		args args
	}{
		{"1", args{1, []byte("213123"), []byte("2342342")}},
		{"2", args{1, []byte(gutils.RandomStringWithLength(1024)), []byte(gutils.RandomStringWithLength(1024))}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &EncryptedData{
				Version:    EncryptedItemVer1,
				KekID:      tt.args.KekID,
				DekID:      tt.args.DekID,
				Ciphertext: tt.args.Ciphertext,
			}

			data, err := e.Marshal()
			require.NoError(t, err)

			e2 := EncryptedData{}
			err = e2.Unmarshal(data)
			require.NoError(t, err)

			require.Equal(t, e.Version, e2.Version)
			require.Equal(t, e.KekID, e2.KekID)
			require.Equal(t, e.DekID, e2.DekID)
			require.Equal(t, e.Ciphertext, e2.Ciphertext)
		})

		t.Run(tt.name, func(t *testing.T) {
			e := &EncryptedData{
				Version:    EncryptedItemVer1,
				KekID:      tt.args.KekID,
				DekID:      tt.args.DekID,
				Ciphertext: tt.args.Ciphertext,
			}

			data, err := e.MarshalToString()
			require.NoError(t, err)

			e2 := EncryptedData{}
			err = e2.UnmarshalFromString(data)
			require.NoError(t, err)

			require.Equal(t, e.Version, e2.Version)
			require.Equal(t, e.KekID, e2.KekID)
			require.Equal(t, e.DekID, e2.DekID)
			require.Equal(t, e.Ciphertext, e2.Ciphertext)
		})
	}

	t.Run("error", func(t *testing.T) {
		e := EncryptedData{}
		err := e.Unmarshal([]byte("123"))
		require.ErrorContains(t, err, "data too short")

		err = e.Unmarshal([]byte(gutils.RandomStringWithLength(256)))
		require.ErrorContains(t, err, "encrypted_item_unimplemented")
	})
}

func TestEncryptedDataVer_String(t *testing.T) {
	tests := []struct {
		name string
		e    EncryptedDataVer
		want string
	}{
		{"1", EncryptedItemVer1, "encrypted_item_ver_1"},
		{"2", EncryptedDataVer(100), "encrypted_item_unimplemented"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.e.String(); got != tt.want {
				t.Errorf("EncryptedDataVer.String() = %v, want %v", got, tt.want)
			}
		})
	}
}
