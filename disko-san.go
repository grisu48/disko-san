/*
 *
 */
package main

import (
	"bufio"
	"fmt"
	"io"
	"math/rand"
	"os"
	"strconv"
	"time"
)

// Program configuration parameters
type config struct {
	disk        string
	stateFile   string // File containing the status of the progress
	throughFile string // File for continous throughput logging
	blockSize   int64  // Number of bytes written in a single burst
}

var cf config

type Disk struct {
	size     int64 // Disk size
	position int64 // Current position
	state    int
}

type ThroughputFile struct {
	filename string
	file     *os.File
}

func (f *ThroughputFile) Open(filename string) error {
	var err error
	f.file, err = os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0640)
	return err
}
func (f *ThroughputFile) Close() error {
	if f.file != nil {
		err := f.file.Close()
		f.file = nil
		return err
	}
	return nil
}

func (f *ThroughputFile) Write(position int64, size int64, time int64) error {
	if f.file == nil {
		return nil
	}
	_, err := f.file.Write([]byte(fmt.Sprintf("%d,%d,%d\n", position, size, time)))
	return err
}

func fileExists(filename string) bool {
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return false
	}
	return true
}

func smooth(val, average, alpha float32) float32 {
	// Initial value
	if average == 0.0 {
		return val
	}
	return alpha*average + (1.0-alpha)*val
}

func readStateFile(filename string) (Disk, error) {
	var disk Disk
	f, err := os.OpenFile(filename, os.O_RDONLY, 0600)
	if err != nil {
		return disk, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	if !scanner.Scan() {
		return disk, fmt.Errorf("Premature file ending")
	}
	if disk.size, err = strconv.ParseInt(scanner.Text(), 10, 64); err != nil {
		return disk, err
	}
	if !scanner.Scan() {
		return disk, fmt.Errorf("Premature file ending")
	}
	if disk.position, err = strconv.ParseInt(scanner.Text(), 10, 64); err != nil {
		return disk, err
	}
	if !scanner.Scan() {
		return disk, fmt.Errorf("Premature file ending")
	}
	if disk.state, err = strconv.Atoi(scanner.Text()); err != nil {
		return disk, err
	}
	return disk, nil
}

func writeStateFile(filename string, disk Disk) error {
	f, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE, 0640)
	if err != nil {
		return err
	}
	defer f.Close()
	str := fmt.Sprintf("%d\n%d\n%d", disk.size, disk.position, disk.state)
	if _, err = f.Write([]byte(str)); err != nil {
		return err
	}
	return f.Sync()
}

func gibistr(bytes float32) string {
	if bytes >= 1024 {
		kb := bytes / 1024.0
		if kb > 1024 {
			mb := kb / 1024.0
			if mb > 1024 {
				gb := mb / 1024.0
				if gb > 1024 {
					tb := gb / 1024
					if tb > 1024 {
						pb := tb / 1024
						return fmt.Sprintf("%.2f PiB", pb)
					}
					return fmt.Sprintf("%.2f TiB", tb)
				}
				return fmt.Sprintf("%.2f GiB", gb)
			}
			return fmt.Sprintf("%.2f MiB", mb)
		}
		return fmt.Sprintf("%.2f kiB", kb)
	}
	return fmt.Sprintf("%.2f B", bytes)
}

/* Run disk check on the given disk */
func checkDisk(disk string) error {
	var tFile ThroughputFile
	if cf.blockSize <= 0 {
		panic(fmt.Errorf("Invalid block size %d", cf.blockSize))
	}
	buf := make([]byte, cf.blockSize)
	var state Disk
	dev, err := os.OpenFile(disk, os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer dev.Close()

	// determine size by seeking at the end of the file
	var size int64
	if size, err = dev.Seek(0, io.SeekEnd); err != nil {
		return err
	}
	if _, err = dev.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if size <= 0 {
		return fmt.Errorf("invalid size %d", size)
	}

	fmt.Printf("Determined size: %s (%d Bytes)\n", gibistr(float32(size)), size)
	fmt.Printf("Block size: %s (%d)\n", gibistr(float32(cf.blockSize)), cf.blockSize)

	// Read disk configuration, if present
	if cf.stateFile != "" {
		if fileExists(cf.stateFile) {
			state, err = readStateFile(cf.stateFile)
			if err != nil {
				return fmt.Errorf("Error loading state file: %s", err)
			}
			// Check if size is matching
			if size != state.size {
				fmt.Fprintf(os.Stderr, "Disk size does not match state file. Disk: %d, state file: %d\n", size, state.size)
				return fmt.Errorf("Disk size mismatch")
			}
			// Already done?
			if state.state != 1 {
				fmt.Fprintf(os.Stderr, "state file defines status %d\n", state.state)
				return fmt.Errorf("invalid state")
			}
			fmt.Printf("Resume from %d (%.2f %%)\n", state.position, 100.0*(float32(state.position)/float32(state.size)))
		} else {
			// Create file
			state.size = size
			state.position = 0
			state.state = 1 // running
			if err := writeStateFile(cf.stateFile, state); err != nil {
				return fmt.Errorf("Error writing state file: %s", err)
			}
		}
	} else {
		state.state = 1 // Running
		state.size = size
	}

	// Seek to the last given position
	pos, err := dev.Seek(state.position, 0)
	if err != nil {
		return err
	}
	if pos != state.position {
		fmt.Fprintf(os.Stderr, "Seek position got %d, expected %d\n", pos, state.position)
		return fmt.Errorf("wrong seek position")
	}

	if cf.throughFile != "" {
		if err := tFile.Open(cf.throughFile); err != nil {
			return fmt.Errorf("Error open throughput file: %s", err)
		}
	}
	defer tFile.Close()

	// Write stuff to disk
	size = int64(len(buf))
	fmt.Printf("\033[s") // save cursor position
	avgThroughput := float32(0.0)
	for state.state == 1 {
		// Buffer might be smaller at the enf of the file
		if state.position+size > state.size {
			size = state.size - state.position
			buf = make([]byte, size)
		}

		// TODO: Make separate read/write thread
		n, err := rand.Read(buf)
		if err != nil {
			return fmt.Errorf("error reading from rand: %s", err)
		}
		if int64(n) != size {
			return fmt.Errorf("didn't got enough bytes from rand (%d<%d)", n, len(buf))
		}

		// Write to disk
		runtime := time.Now().UnixNano()
		n, err = dev.Write(buf)
		if err != nil {
			return fmt.Errorf("error writing to disk: %s", err)
		}
		if err = dev.Sync(); err != nil {
			return fmt.Errorf("error synching disk: %s", err)
		}
		runtime = (time.Now().UnixNano() - runtime)
		if int64(n) != size {
			return fmt.Errorf("Didn't wrote enough bytes to disk: %d < %d", n, len(buf))
		}

		// Print current status
		throughput := (float32(size) / float32(runtime)) * 1e9
		if runtime == 0 { // Don't know how to handle this
			throughput = 0
		}
		percent := 100.0 * (float32(state.position) / float32(state.size))
		fmt.Printf("\033[u") // restore cursor position
		fmt.Printf("\033[K") // erase rest of line
		avgThroughput = smooth(throughput, avgThroughput, 0.9)
		fmt.Printf("%.2f %% done (%d/%d) - Throughput: %s/s", percent, state.position, state.size, gibistr(avgThroughput))
		if cf.throughFile != "" {
			// Ignore errors
			tFile.Write(state.position, size, runtime)
		}

		// Update state file
		state.position += size
		if state.position >= state.size {
			state.state = 2 // done
			break
		}
		if cf.stateFile != "" {
			if err := writeStateFile(cf.stateFile, state); err != nil {
				return fmt.Errorf("error writing state file: %s", err)
			}
		}
	}

	if state.state != 2 {
		return fmt.Errorf("state %d", state.state)
	}
	return nil
}

func main() {
	cf.disk = ""
	cf.stateFile = ""
	cf.blockSize = 4 * 1024 * 1024 // Write at once size (4 MiB by default)

	// TOOD: Better argument handling
	if len(os.Args) < 2 {
		fmt.Printf("Usage: %s DISK [STATE] [SPEEDLOG]\n", os.Args[0])
		fmt.Println("    DISK:      Disk file under test")
		fmt.Println("    STATE:     State file for test continue")
		fmt.Println("    SPEEDLOG:  Write disk throughput log to here")
		os.Exit(1)
	}
	if len(os.Args) >= 2 {
		cf.disk = os.Args[1]
	}
	if len(os.Args) > 2 {
		cf.stateFile = os.Args[2]
	}
	if len(os.Args) > 3 {
		cf.throughFile = os.Args[3]
	}

	if cf.disk == "" {
		fmt.Fprintf(os.Stderr, "Missing disk\n")
		os.Exit(1)
	}

	// Run disk check of configured disk
	if err := checkDisk(cf.disk); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
	fmt.Println("")
	fmt.Println("Done")
}
