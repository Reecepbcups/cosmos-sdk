package ethsecp256k1

import (
	"bytes"
	"crypto/ecdsa"

	ethcrypto "github.com/cosmos/cosmos-sdk/crypto/keys/eth_crypto"
	"github.com/cosmos/cosmos-sdk/crypto/keys/eth_crypto/secp256k1"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"

	tmcrypto "github.com/cometbft/cometbft/crypto"
)

const (
	// PrivKeySize defines the size of the PrivKey bytes
	PrivKeySize = 32
	// KeyType is the string constant for the EthSecp256k1 algorithm
	KeyType = "ethsecp256k1"
)

// Amino encoding names
const (
	// PrivKeyName defines the amino encoding name for the EthSecp256k1 private key
	PrivKeyName = "ethermint/PrivKeyEthSecp256k1"
	// PubKeyName defines the amino encoding name for the EthSecp256k1 public key
	PubKeyName = "ethermint/PubKeyEthSecp256k1"
)

// ----------------------------------------------------------------------------
// ethsecp256k1 Private Key

var _ cryptotypes.PrivKey = (*PrivKey)(nil)

// PrivKey defines a type alias for an ecdsa.PrivateKey that implements
// Tendermint's PrivateKey interface.
// type PrivKey []byte
type PrivKey struct {
	Key []byte `protobuf:"bytes,1,opt,name=key,proto3" json:"key,omitempty"`
}

// Type implements crypto.PrivKey.
func (PrivKey) Type() string {
	return KeyType
}

// GenerateKey generates a new random private key. It returns an error upon
// failure.
func GenerateKey() (PrivKey, error) {
	priv, err := ethcrypto.GenerateKey()
	if err != nil {
		return PrivKey{}, err
	}

	return PrivKey{Key: ethcrypto.FromECDSA(priv)}, nil
}

// PubKey returns the ECDSA private key's public key.
func (privkey PrivKey) PubKey() cryptotypes.PubKey {
	ecdsaPKey := privkey.ToECDSA()
	return PubKey(ethcrypto.CompressPubkey(&ecdsaPKey.PublicKey))
}

// Bytes returns the raw ECDSA private key bytes.
func (privkey PrivKey) Bytes() []byte {
	// return CryptoCodec.MustMarshalBinaryBare(privkey)
	return privkey.Key
}

// Sign creates a recoverable ECDSA signature on the secp256k1 curve over the
// Keccak256 hash of the provided message. The produced signature is 65 bytes
// where the last byte contains the recovery ID.
func (privkey PrivKey) Sign(msg []byte) ([]byte, error) {
	return ethcrypto.Sign(ethcrypto.Keccak256Hash(msg).Bytes(), privkey.ToECDSA())
}

// Equals returns true if two ECDSA private keys are equal and false otherwise.
func (privkey PrivKey) Equals(other cryptotypes.LedgerPrivKey) bool {
	if other, ok := other.(PrivKey); ok {
		return bytes.Equal(privkey.Bytes(), other.Bytes())
	}

	return false
}

func (m *PrivKey) Reset()         { *m = PrivKey{} }
func (m *PrivKey) String() string { return "EthSecp256k1" }
func (*PrivKey) ProtoMessage()    {}

// ToECDSA returns the ECDSA private key as a reference to ecdsa.PrivateKey type.
// The function will panic if the private key is invalid.
func (privkey PrivKey) ToECDSA() *ecdsa.PrivateKey {
	key, err := ethcrypto.ToECDSA(privkey.Key)
	if err != nil {
		panic(err)
	}
	return key
}

// ----------------------------------------------------------------------------
// secp256k1 Public Key

var _ cryptotypes.PubKey = (*PubKey)(nil)

// PubKey defines a type alias for an ecdsa.PublicKey that implements Tendermint's PubKey
// interface. It represents the 33-byte compressed public key format.
type PubKey []byte

// Address returns the address of the ECDSA public key.
// The function will panic if the public key is invalid.
func (key PubKey) Address() tmcrypto.Address {
	pubk, err := ethcrypto.DecompressPubkey(key)
	if err != nil {
		panic(err)
	}

	return tmcrypto.Address(ethcrypto.PubkeyToAddress(*pubk).Bytes())
}

// Bytes returns the raw bytes of the ECDSA public key.
// The function panics if the key cannot be marshaled to bytes.
func (key PubKey) Bytes() []byte {
	// bz, err := CryptoCodec.MarshalBinaryBare(key)
	// if err != nil {
	// 	panic(err)
	// }
	// return bz
	return key
}

// Type implements crypto.PubKey.
func (PubKey) Type() string {
	return KeyType // TODO:?
}

// VerifySignature implements crypto.PubKey.
// VerifyBytes verifies that the ECDSA public key created a given signature over
// the provided message. It will calculate the Keccak256 hash of the message
// prior to verification.
func (key PubKey) VerifySignature(msg []byte, sig []byte) bool {
	if len(sig) == 65 {
		// remove recovery ID if contained in the signature
		sig = sig[:len(sig)-1]
	}

	// the signature needs to be in [R || S] format when provided to VerifySignature
	return secp256k1.VerifySignature(key.Bytes(), ethcrypto.Keccak256Hash(msg).Bytes(), sig)
}

// Equals returns true if two ECDSA public keys are equal and false otherwise.
func (key PubKey) Equals(other cryptotypes.PubKey) bool {
	return bytes.Equal(key.Bytes(), other.Bytes())
}

// ProtoMessage implements types.PubKey.
func (PubKey) ProtoMessage() {
}

// Reset implements types.PubKey.
func (pk PubKey) Reset() {
}

// String implements types.PubKey.
func (PubKey) String() string {
	return ""
}
