package simhash

import (
	"encoding/base64"
	"math/big"

	"golang.org/x/crypto/blake2b"
)

type Simhash struct {
	Size  int
	Value *big.Int
}

func GetSimhash(features map[string]int, size int) string {
	simhash := generateSimhash(features, size)
	return encodeSimhashToBase64(simhash, size)
}

func hashFunc(data string) *big.Int {
	hasher, _ := blake2b.New512(nil)
	hasher.Write([]byte(data))
	return new(big.Int).SetBytes(hasher.Sum(nil))
}

func generateSimhash(features map[string]int, size int) *big.Int {
	vector := make([]int, size)
	mask := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), uint(size)), big.NewInt(1))

	for k, v := range features {
		if v <= 0 {
			continue
		}
		h := hashFunc(k)
		h.And(h, mask)

		for i := 0; i < size; i++ {
			if h.Bit(i) == 1 {
				vector[i] += v
			} else {
				vector[i] -= v
			}
		}
	}

	value := big.NewInt(0)
	for i := 0; i < size; i++ {
		if vector[i] > 0 {
			value.SetBit(value, i, 1)
		}
	}

	return value
}

func packSimhashToBytes(simhash *big.Int, size int) []byte {
	sizeInBytes := size / 8
	bytes := simhash.FillBytes(make([]byte, sizeInBytes))

	// Convert to Little Endian
	for i, j := 0, len(bytes)-1; i < j; i, j = i+1, j-1 {
		bytes[i], bytes[j] = bytes[j], bytes[i]
	}

	return bytes
}

func encodeSimhashToBase64(simhash *big.Int, size int) string {
	return base64.StdEncoding.EncodeToString(packSimhashToBytes(simhash, size))
}
