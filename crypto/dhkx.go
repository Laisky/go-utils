package crypto

import (
	"github.com/Laisky/errors"
	"github.com/monnand/dhkx"
)

// Diffie Hellman Key-exchange algorithm
//
// https://pkg.go.dev/github.com/monnand/dhkx
//
// # Example
//
//	alice, _ := NewDHKX()
//	bob, _ := NewDHKX()
//
//	alicePub := alice.PublicKey()
//	bobPub := bob.PublicKey()
//
//	aliceKey, _ := alice.GenerateKey(bobPub)
//	bobKey, _ := bob.GenerateKey(alicePub)
//
//	// aliceKey == bobKey
type DHKX struct {
	g    *dhkx.DHGroup
	priv *dhkx.DHKey
}

type dhkxOption struct {
	group int
}

func (o *dhkxOption) fillDefault() *dhkxOption {
	return o
}

func (o *dhkxOption) applyOpts(opts ...DHKXOptionFunc) (*dhkxOption, error) {
	for _, opt := range opts {
		if err := opt(o); err != nil {
			return nil, err
		}
	}

	return o, nil
}

// DHKXOptionFunc optional func to set dhkx option
type DHKXOptionFunc func(*dhkxOption) error

// NewDHKX create a new DHKX instance
//
// each DHKX instance has it's unique group and private key
func NewDHKX(optfs ...DHKXOptionFunc) (d *DHKX, err error) {
	opt, err := new(dhkxOption).fillDefault().applyOpts(optfs...)
	if err != nil {
		return nil, err
	}

	d = new(DHKX)
	if d.g, err = dhkx.GetGroup(opt.group); err != nil {
		return nil, errors.Wrap(err, "get group")
	}

	if d.priv, err = d.g.GeneratePrivateKey(nil); err != nil {
		return nil, errors.Wrap(err, "generate key")
	}

	return d, nil
}

// PublicKey return public key bytes
func (d *DHKX) PublicKey() []byte {
	return d.priv.Bytes()
}

// GenerateKey generate new key by peer's public key
//
// each side of the DHKX exchange peers will generate the same key
//
// key like:
//
//	60a425ca3a4cc313db9c113a0526f3809725305afc68e1accd0e653ae8d0182c6eb05557f4b5d094
//	f015972b9fda7d60c1b64d79f50baea7365d858ede0fb7a6571403d4b95f682144b56fa17ffcbe9e
//	70de69dc0045672696e683c423c5b3dfc02a6916be1e50c74e60353ec08a465cc124e8ca88337fb7
//	4a0370e17a7cedb0b1e76733f43ad3db9e3d29ab43c75686a8bc4a88ee46addbd1590c8277d1b1ef
//	42aded6cc0bfe0a7ff8933861dae772c755087f2a41021f4ca53867ba49797d111ef21b381cb6441
//	178f4ccd3748f8e7b1a12ec3799571a49fc0aa793c05ab6e228b559f1fda2912542d7246388ccec1
//	38b4d8ce9df4a32c198891c4e33b5034
func (d *DHKX) GenerateKey(peerPubKey []byte) ([]byte, error) {
	k, err := d.g.ComputeKey(dhkx.NewPublicKey(peerPubKey), d.priv)
	if err != nil {
		return nil, errors.Wrap(err, "compute key")
	}

	return k.Bytes(), nil
}
