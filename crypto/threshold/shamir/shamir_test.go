package shamir

import (
	"testing"

	"github.com/stretchr/testify/require"

	gutils "github.com/Laisky/go-utils/v4"
)

func TestSplit(t *testing.T) {
	type args struct {
		secret    []byte
		total     int
		threshold int
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{"0", args{[]byte(gutils.RandomStringWithLength(1024)), 10, 5}, false},
		{"1", args{[]byte(gutils.RandomStringWithLength(1024)), 20, 10}, false},
		{"2", args{[]byte(gutils.RandomStringWithLength(1024)), 35, 30}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			members, err := Split(tt.args.secret, tt.args.total, tt.args.threshold)
			require.NoError(t, err)

			var ks []byte
			for b := range members {
				ks = append(ks, b)
			}

			randor := gutils.NewRand()

			t.Run("fulfill threshold", func(t *testing.T) {
				for i := 0; i < 10; i++ {
					k := randor.Intn(tt.args.total-tt.args.threshold) + tt.args.threshold

					parts := map[byte][]byte{}
					for _, b := range gutils.RandomChoice(ks, k) {
						parts[b] = members[b]
					}

					cipher, err := Combine(parts)
					t.Logf("total: %d, threshold: %d, parts: %d", tt.args.total, tt.args.threshold, k)
					require.NoError(t, err)
					require.Equal(t, tt.args.secret, cipher)
				}
			})

			t.Run("less than threshold", func(t *testing.T) {
				for i := 0; i < 10; i++ {
					k := randor.Intn(tt.args.threshold)
					if k < 2 {
						k = 2
					}

					parts := map[byte][]byte{}
					for _, b := range gutils.RandomChoice(ks, k) {
						parts[b+33] = members[b]
					}

					cipher, err := Combine(parts)
					t.Logf("total: %d, threshold: %d, parts: %d", tt.args.total, tt.args.threshold, k)
					require.NoError(t, err)
					require.NotEqual(t, tt.args.secret, cipher)
				}
			})

		})
	}
}
