package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// Program configuration parameters
type conf struct {
	disk     string
	progress string // Progress file for continue the job later on
	stats    string // Performance log
	verbose  bool
}

var cf conf
var avg float32    // average for average smoothing
var running bool   // running flag
var done chan bool // Signal for when the main thread is completed

func (cf *conf) CheckValid() error {

	if cf.disk == "" {
		return fmt.Errorf("missing disk file")
	}
	return nil
}

func fileExists(filename string) bool {
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return false
	}
	return true
}

func smooth(val float32, alpha float32) float32 {
	if avg == 0 {
		avg = val
	} else {
		avg = alpha*avg + (1.0-alpha)*val
	}
	return avg
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

func bufCompare(a []byte, b []byte) bool {
	n := len(a)
	if len(b) != n {
		return false
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

/* Check the internal functions.
 * We write the first chunk and check if it verifies, then we corrupt it and check if the verification fails
 */
func CheckInternals(disk *Disk) error {
	var n int
	var err error
	chunk := make([]byte, CHUNKSIZE)

	pos, err := disk.Position()
	if err != nil {
		return err
	}
	defer disk.Seek(pos) // Return original disk position at the end

	if err = disk.Seek(CHUNKSIZE); err != nil {
		return err
	}
	CreateChunk(chunk)
	if !VerifyChunk(chunk) {
		return fmt.Errorf("chunk verification function failed")
	}
	if n, err = disk.Write(chunk); err != nil {
		return err
	} else if n < CHUNKSIZE { // Suspicious: First trunk is already truncated?
		fmt.Fprintf(os.Stderr, "Warning: First chunk already truncated\n")
		chunk = chunk[:n]
	}
	if err := disk.Sync(); err != nil {
		return err
	}

	// Now read the chunk, it must be the same
	buf := make([]byte, n)
	if err := disk.Seek(CHUNKSIZE); err != nil { // Move back to where we wrote before
		return err
	}
	if n, err := disk.Read(buf); err != nil {
		return err
	} else if n != len(buf) {
		fmt.Fprintf(os.Stderr, "Read chunk size (%d) is not the same size as write chunk size (%d)\n", n, len(buf))
		return fmt.Errorf("chunk size mismatch")
	}
	// Primitive check, if the read buffer compares to the written buffer
	if !bufCompare(buf, chunk) {
		return fmt.Errorf("read chunk is not the same as the written chunk")
	}
	// In principle a redundant check, but it does not hurt here
	if !VerifyChunk(buf) {
		return fmt.Errorf("read chunk verification check failed")
	}

	// Now we do something nasty. Corrupt the chunk, write it again and then the verification must fail
	chunk[5]++ // corrupt chunk
	if VerifyChunk(chunk) {
		return fmt.Errorf("chunk verification passed after corruption")
	}
	if err := disk.Seek(CHUNKSIZE); err != nil { // Move back to first chunk
		return err
	}
	if n, err = disk.Write(chunk); err != nil {
		return err
	} else if n != len(chunk) { // This should never happen here again!!
		return fmt.Errorf("write buffer decreased")
	}
	if err := disk.Seek(CHUNKSIZE); err != nil { // Move back to first chunk
		return err
	}
	if n, err := disk.Read(buf); err != nil {
		return err
	} else if n != len(buf) {
		fmt.Fprintf(os.Stderr, "Read chunk size (%d) is not the same size as write chunk size (%d)\n", n, len(buf))
		return fmt.Errorf("chunk size mismatch")
	}
	// Primitive check, if the read buffer compares to the written buffer
	if !bufCompare(buf, chunk) {
		return fmt.Errorf("read chunk is not the same as the written chunk")
	}
	// In principle again redundant check, but it does not hurt here
	if VerifyChunk(buf) {
		return fmt.Errorf("corrupted chunk reports valid verification")
	}

	// Important: Restore a valid chunk otherwise resume will fail because disk contains now a invalid chunk at position 1
	CreateChunk(chunk)
	if !VerifyChunk(chunk) {
		return fmt.Errorf("chunk verification function failed")
	}
	if err := disk.Seek(CHUNKSIZE); err != nil { // Move back to first chunk
		return err
	}
	if n, err = disk.Write(chunk); err != nil {
		return err
	} else if n < CHUNKSIZE { // Suspicious: First trunk is already truncated?
		fmt.Fprintf(os.Stderr, "Warning: First chunk already truncated\n")
		chunk = chunk[:n]
	}
	if err := disk.Sync(); err != nil {
		return err
	}

	return nil
}

/* Do the write check*/
func WriteCheck(disk *Disk, progress *Progress, statsFile string) error {
	var stats *os.File // stats file, if present
	chunk := make([]byte, CHUNKSIZE)

	if statsFile != "" {
		var err error
		exists := fileExists(statsFile)
		stats, err = os.OpenFile(statsFile, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0640)
		if err != nil {
			return fmt.Errorf("Error opening stats file : %s", err)
		}
		// Write stats file header only once
		if !exists {
			if _, err := stats.Write([]byte("# disko-san performance metrics file\nPosition [B], Size [B], Millis [ms]\n\n")); err != nil {
				return fmt.Errorf("Error writing header to stats file : %s", err)
			}
		}
		defer stats.Close()
	}

	// Move to position
	if progress.Pos == 0 {
		progress.Pos = CHUNKSIZE // First chunk contains magic, skip it
	}
	if err := disk.Seek(progress.Pos); err != nil {
		return err
	}

	// Background chunk production instance
	var cf ChunkFactory
	cf.StartProduce(CHUNKSIZE)
	defer cf.Stop()

	fmt.Printf("\033[s") // save cursor position
	for progress.Pos < progress.Size {
		if !running {
			return fmt.Errorf("interrupted")
		}
		// Determine size of current chunk - at the end of the disk this might not be the full size anymore
		size := int64(CHUNKSIZE)
		if progress.Pos+CHUNKSIZE > progress.Size {
			size = progress.Size - progress.Pos
			chunk = chunk[:size]
		}

		// Create chunk
		if err := cf.Read(chunk); err != nil {
			return fmt.Errorf("ChunkFactory read error: %s", err)
		}
		// Write chunk to file with runtime
		runtime := time.Now().UnixNano()
		if n, err := disk.Write(chunk); err != nil {
			return err
		} else {
			size = int64(n)
		}
		if err := disk.Sync(); err != nil {
			return err
		}
		runtime = time.Now().UnixNano() - runtime
		millis := runtime / 1e6

		// Write performance stats
		if statsFile != "" {
			line := fmt.Sprintf("%d,%d,%d\n", progress.Pos, size, millis)
			if _, err := stats.Write([]byte(line)); err != nil {
				return fmt.Errorf("Error writing to stats file: %s", err)
			}
		}

		// Update progress
		progress.Pos += size
		if err := progress.WriteIfOpen(); err != nil {
			return fmt.Errorf("Error writing progress file: %s", err)
		}

		// Compute throughput and print update
		throughput := (float32(size) / float32(millis)) * 1e3

		fmt.Printf("\033[u") // restore cursor position
		fmt.Printf("\033[K") // erase rest of line
		percent := 100.0 * (float32(progress.Pos) / float32(disk.Size()))
		fmt.Printf("Writing chunks: %.2f %% done @ %s/s", percent, gibistr(smooth(throughput, 0.75)))
	}

	fmt.Printf("\033[u") // restore cursor position
	fmt.Printf("\033[K") // erase rest of line
	fmt.Println("Write test successful")

	return nil
}

/* Do the read check*/
func ReadCheck(disk *Disk, progress *Progress) error {
	chunk := make([]byte, CHUNKSIZE)

	// Move to position
	if progress.Pos == 0 {
		progress.Pos = CHUNKSIZE // First chunk contains magic, skip it
	}
	if err := disk.Seek(progress.Pos); err != nil {
		return err
	}

	// Read chunks one by one and verify them
	fmt.Printf("\033[s") // save cursor position
	for progress.Pos < progress.Size {
		if !running {
			return fmt.Errorf("interrupted")
		}
		// Read and verify chunk
		runtime := time.Now().UnixNano()
		n, err := disk.Read(chunk)
		runtime = time.Now().UnixNano() - runtime
		if err != nil {
			return err
		} else if n < len(chunk) { // at the end of the disk, the chunk might be smaller
			chunk = chunk[:n]
		}
		if !VerifyChunk(chunk) {
			fmt.Println()
			fmt.Fprintf(os.Stderr, "Chunk %d verification error (disk position %d)\n", progress.Pos/CHUNKSIZE, progress.Pos)
			return err
		}

		// Update progress
		progress.Pos += int64(n)
		if err := progress.WriteIfOpen(); err != nil {
			return fmt.Errorf("Error writing progress file: %s", err)
		}

		// Print stats
		millis := runtime / 1e6
		throughput := (float32(n) / float32(millis)) * 1e3
		fmt.Printf("\033[u") // restore cursor position
		fmt.Printf("\033[K") // erase rest of line
		percent := 100.0 * (float32(progress.Pos) / float32(disk.Size()))
		fmt.Printf("Reading chunks: %.2f %% done @ %s/s", percent, gibistr(smooth(throughput, 0.75)))
	}

	fmt.Printf("\033[u") // restore cursor position
	fmt.Printf("\033[K") // erase rest of line
	fmt.Println("Read test successful")

	return nil
}

func printUsage() {
	fmt.Printf("Usage: %s DISK [PROGRESS] [SPEEDLOG]\n", os.Args[0])
	fmt.Println("    DISK:         Disk file under test")
	fmt.Println("    PROGRESS:     Progress file, required for job continuation")
	fmt.Println("    SPEEDLOG:     Performance metrics log")
}

func parseArgs(args []string, cf *conf) error {
	if len(args) < 2 {
		printUsage()
		os.Stdout.Sync() // Ensure usage is flushed to stdout before returning with an error
		return fmt.Errorf("Missing arguments")
	}
	if len(args) >= 2 {
		cf.disk = args[1]
		if cf.disk == "-h" || cf.disk == "--help" {
			printUsage()
			os.Exit(0)
		}
	}
	if len(args) >= 3 {
		cf.progress = args[2]
	}
	if len(args) >= 4 {
		cf.stats = args[3]
	}
	if len(args) > 4 {
		return fmt.Errorf("too many arguments")
	}
	return nil
}

func terminationSignalHandler() {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigs
	fmt.Println(sig)
	running = false
	// Wait for termination signal but quit after 2 seconds unconditionally
	select {
	case <-done:
		os.Exit(1)
	case <-time.After(2 * time.Second):
		fmt.Fprintf(os.Stderr, "Termination timeout. Forcefully quiting.\n")
		os.Exit(1)
	}
	// Just to be sure
	os.Exit(1)
}

func main() {
	var progress Progress
	done = make(chan bool, 1)
	running = true

	// Default settings
	cf.disk = ""
	cf.progress = ""
	cf.stats = ""
	cf.verbose = false

	if err := parseArgs(os.Args, &cf); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}

	// Check configuration for validity
	if err := cf.CheckValid(); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}

	// Prepare disk
	disk := CreateDisk(cf.disk)
	if err := disk.Open(); err != nil {
		fmt.Fprintf(os.Stderr, "Error opening disk: %s\n", err)
		os.Exit(1)
	}
	defer disk.Close()

	// Load progress stats if present
	if cf.progress != "" {
		if fileExists(cf.progress) {
			if err := progress.Open(cf.progress); err != nil {
				fmt.Fprintf(os.Stderr, "Error opening progress file %s: %s\n", cf.progress, err)
				os.Exit(1)
			}
			if err := progress.Read(); err != nil {
				fmt.Fprintf(os.Stderr, "Error reading progress file %s: %s\n", cf.progress, err)
				os.Exit(1)
			}
			if progress.State == 0 {
				fmt.Printf("Resume operation on disk\n")
			} else if progress.State == 1 {
				percent := 100.0 * (float32(progress.Pos) / float32(disk.Size()))
				fmt.Printf("Resuming write test at %d (%.2f %% already done)\n", progress.Pos, percent)
			} else if progress.State == 2 {
				percent := 100.0 * (float32(progress.Pos) / float32(disk.Size()))
				fmt.Printf("Resuming read test at %d (%.2f %% already done)\n", progress.Pos, percent)
			} else if progress.State == 3 {
				fmt.Println("Disk already completed. Nothing to be done")
				os.Exit(0)
			} else {
				fmt.Fprintf(os.Stderr, "Invalid progress state %d\n", progress.State)
				os.Exit(1)
			}
		} else {
			// Create file
			if err := progress.Open(cf.progress); err != nil {
				fmt.Fprintf(os.Stderr, "Error creating progress file %s: %s\n", cf.progress, err)
				os.Exit(1)
			}
			// Defaults
			progress.Pos = 0
			progress.State = 0
			progress.Size = disk.Size()
			if err := progress.Write(); err != nil {
				fmt.Fprintf(os.Stderr, "Error writing to new progress file %s: %s\n", cf.progress, err)
				os.Exit(1)
			}
		}
	} else {
		// Set progress values to defaults
		progress.Size = disk.Size()
		progress.Pos = 0
		progress.State = 0
	}

	if disk.Size() <= 0 {
		fmt.Fprintf(os.Stderr, "Invalid disk size %d\n", disk.Size())
		os.Exit(1)
	}

	// Check program internals before each run.
	if err := CheckInternals(&disk); err != nil {
		fmt.Fprintf(os.Stderr, "FATAL ERROR: Pre-flight checks failed. This is a program error, please report a bug!\n")
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(42)
	}
	// Perform disk pre-flight checks, if we continue from a disk
	if cf.progress != "" {
		if progress.State < 0 || progress.State > 3 {
			fmt.Fprintf(os.Stderr, "Invalid progress state %d\n", progress.State)
			os.Exit(1)
		}

		if disk.Size() != progress.Size {
			fmt.Fprintf(os.Stderr, "Error: disk size mismatch\n")
			fmt.Fprintf(os.Stderr, "The disk reports %d bytes, but the progress file says it should be %d (wrong disk?)\n", disk.Size(), progress.Size)
			os.Exit(1)
		}
		// Disk magic check only after preparation step
		if progress.State > 0 {
			if err := disk.CheckMagic(); err != nil {
				fmt.Fprintf(os.Stderr, "Disk magic error: %s\n", err)
				os.Exit(1)
			}
		}
	} else {
		progress.Size = disk.Size()
	}

	// Termination signal handler
	go terminationSignalHandler()

	// Preparation step
	if progress.State == 0 {
		// Prepare disk
		if err := disk.Prepare(); err != nil {
			fmt.Fprintf(os.Stderr, "Disk preparation error: %s\n", err)
			os.Exit(10)
		}
		progress.State = 1
		progress.Pos = 0
		if err := progress.WriteIfOpen(); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing progress file: %s\n", err)
			os.Exit(1)
		}
	}

	// Write step
	if progress.State == 1 {
		if err := WriteCheck(&disk, &progress, cf.stats); err != nil {
			if err.Error() == "interrupted" {
				done <- true
				fmt.Fprintf(os.Stderr, "Cancelled\n")
			} else {
				fmt.Fprintf(os.Stderr, "Write check failed: %s\n", err)
			}
			os.Exit(11)
		}
		progress.State = 2
		progress.Pos = 0
		if err := progress.WriteIfOpen(); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing progress file: %s\n", err)
			os.Exit(1)
		}
	}

	// Read step
	if progress.State == 2 {
		if err := ReadCheck(&disk, &progress); err != nil {
			if err.Error() == "interrupted" {
				done <- true
				fmt.Fprintf(os.Stderr, "Cancelled\n")
			} else {
				fmt.Fprintf(os.Stderr, "Read check failed: %s\n", err)
			}
			os.Exit(12)
		}
		progress.State = 3
		if err := progress.WriteIfOpen(); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing progress file: %s\n", err)
			os.Exit(1)
		}
	}

	// All good
	done <- true
	fmt.Println("Done")
}
