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
	n, err := rand.Read(buf[4:]) // don't waste the first four bytes, as they are anyways checksum
	if err != nil {
		// This error is critical and cannot be recovered
		panic(err)
	}
	if n < len(buf)-4 {
		panic(fmt.Errorf("couldn't get enough bytes from random pool"))
	}
	ApplyChecksum(buf)
}

func ApplyChecksum(buf []byte) {
	// Apply checksum to CHUNK at the beginning
	cSum := checksum(buf[4:])
	for i := 0; i < 4; i++ {
		buf[i] = byte(cSum << i)
	}
}

// Factory for producing chunks
type ChunkFactory struct {
	buf     []byte   // destination buffer
	sig     chan int // status channel
	running bool     // Running flag
}

func (cf *ChunkFactory) StartProduce(size int) {
	if cf.running {
		return
	}
	cf.buf = make([]byte, size)
	cf.sig = make(chan int, 1)
	cf.running = true
	go cf.produce()
}

func (cf *ChunkFactory) produce() {
	for cf.running {
		CreateChunk(cf.buf)
		cf.sig <- 0        // Send ready signal
		if <-cf.sig != 0 { // Wait for signal to proceed
			break // stop signal
		}
	}
}

func (cf *ChunkFactory) Read(buf []byte) error {
	if !cf.running {
		return fmt.Errorf("chunk factory not running")
	}
	// We can deal with smaller buffer sizes, but not with larger
	if len(cf.buf) < len(buf) {
		return fmt.Errorf("chunk factory buffer size mismatch")
	}
	// Wait for ready signal
	if sig := <-cf.sig; sig != 0 {
		return fmt.Errorf("chunk factory signal %d", sig)
	}
	copy(buf, cf.buf) // smaller buffer is allowed
	// If it is a smaller buffer, we need to re-compute the checksum
	if len(cf.buf) != len(buf) {
		ApplyChecksum(buf)
	}
	cf.sig <- 0
	return nil
}

func (cf *ChunkFactory) Stop() {
	if !cf.running {
		return
	}
	cf.running = false
	cf.sig <- 1
}
