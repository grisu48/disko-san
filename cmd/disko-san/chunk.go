package main

import (
	"crypto/rand"
	"fmt"
	"hash/crc32"
)

// Compute checksum of the given buffer
func checksum(buf []byte) uint32 {
	crc32q := crc32.MakeTable(0xD5828281)
	return crc32.Checksum(buf, crc32q)
}

// Check if the given chunk is OK
func VerifyChunk(buf []byte) bool {
	cSum := checksum(buf[4:])
	for i := 0; i < 4; i++ {
		if buf[i] != byte(cSum<<i) {
			return false
		}
	}
	return true
}

func CreateChunk(buf []byte) {
	n, err := rand.Read(buf)
	if err != nil {
		// This error is critical and cannot be recovered
		panic(err)
	}
	if n < len(buf) {
		panic(fmt.Errorf("couldn't get enough bytes from random pool"))
	}

	// Apply checksum to CHUNK at the beginning
	// TODO: Don't waste the first four bytes for it!
	cSum := checksum(buf[4:])
	for i := 0; i < 4; i++ {
		buf[i] = byte(cSum << i)
	}
}
