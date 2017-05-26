package rfc3961

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"github.com/jcmturner/gokrb5/crypto/etype"
	"golang.org/x/crypto/pbkdf2"
)

const (
	s2kParamsZero = 4294967296
	prfconstant   = "prf"
)

// RFC 3961: DR(Key, Constant) = k-truncate(E(Key, Constant, initial-cipher-state)).
//
// key: base key or protocol key. Likely to be a key from a keytab file.
//
// usage: a constant.
//
// n: block size in bits (not bytes) - note if you use something like aes.BlockSize this is in bytes.
//
// k: key length / key seed length in bits. Eg. for AES256 this value is 256.
//
// e: the encryption etype function to use.
func DeriveRandom(key, usage []byte, e etype.EType) ([]byte, error) {
	n := e.GetCypherBlockBitLength()
	k := e.GetKeySeedBitLength()
	//Ensure the usage constant is at least the size of the cypher block size. Pass it through the nfold algorithm that will "stretch" it if needs be.
	nFoldUsage := Nfold(usage, n)
	//k-truncate implemented by creating a byte array the size of k (k is in bits hence /8)
	out := make([]byte, k/8)

	/*If the output	of E is shorter than k bits, it is fed back into the encryption as many times as necessary.
	The construct is as follows (where | indicates concatentation):

	K1 = E(Key, n-fold(Constant), initial-cipher-state)
	K2 = E(Key, K1, initial-cipher-state)
	K3 = E(Key, K2, initial-cipher-state)
	K4 = ...

	DR(Key, Constant) = k-truncate(K1 | K2 | K3 | K4 ...)*/
	_, K, err := e.EncryptData(key, nFoldUsage)
	if err != nil {
		return out, err
	}
	for i := copy(out, K); i < len(out); {
		_, K, _ = e.EncryptData(key, K)
		i = i + copy(out[i:], K)
	}
	return out, nil
}

func DeriveKey(protocolKey, usage []byte, e etype.EType) ([]byte, error) {
	r, err := DeriveRandom(protocolKey, usage, e)
	if err != nil {
		return nil, err
	}
	return RandomToKey(r), nil
}

func RandomToKey(b []byte) []byte {
	return b
}

func StringToKey(secret, salt, s2kparams string, e etype.EType) ([]byte, error) {
	i, err := S2KparamsToItertions(s2kparams)
	if err != nil {
		return nil, err
	}
	return StringToKeyIter(secret, salt, int(i), e)
}

func S2KparamsToItertions(s2kparams string) (int, error) {
	//process s2kparams string
	//The parameter string is four octets indicating an unsigned
	//number in big-endian order.  This is the number of iterations to be
	//performed.  If the value is 00 00 00 00, the number of iterations to
	//be performed is 4,294,967,296 (2**32).
	var i uint32
	if len(s2kparams) != 8 {
		return s2kParamsZero, errors.New("Invalid s2kparams length")
	}
	b, err := hex.DecodeString(s2kparams)
	if err != nil {
		return s2kParamsZero, errors.New("Invalid s2kparams, cannot decode string to bytes")
	}
	i = binary.BigEndian.Uint32(b)
	//buf := bytes.NewBuffer(b)
	//err = binary.Read(buf, binary.BigEndian, &i)
	if err != nil {
		return s2kParamsZero, errors.New("Invalid s2kparams, cannot convert to big endian int32")
	}
	return int(i), nil
}

func StringToPBKDF2(secret, salt string, iterations int, e etype.EType) []byte {
	return pbkdf2.Key([]byte(secret), []byte(salt), iterations, e.GetKeyByteSize(), e.GetHash())
}

func StringToKeyIter(secret, salt string, iterations int, e etype.EType) ([]byte, error) {
	tkey := RandomToKey(StringToPBKDF2(secret, salt, iterations, e))
	return DeriveKey(tkey, []byte("kerberos"), e)
}

func IterationsToS2kparams(i int) string {
	b := make([]byte, 4, 4)
	binary.BigEndian.PutUint32(b, uint32(i))
	return hex.EncodeToString(b)
}

func PseudoRandom(key, b []byte, e etype.EType) ([]byte, error) {
	h := e.GetHash()()
	h.Write(b)
	tmp := h.Sum(nil)[:e.GetMessageBlockByteSize()]
	k, err := DeriveKey(key, []byte(prfconstant), e)
	if err != nil {
		return []byte{}, err
	}
	_, prf, err := EncryptData(k, tmp, e)
	if err != nil {
		return []byte{}, err
	}
	return prf, nil
}
