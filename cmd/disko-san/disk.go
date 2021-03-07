/* Disk handling for disko-san */
package main

import (
	"fmt"
	"io"
	"os"
)

const CHUNKSIZE = 4 * 1024 * 1024                                // Chunk size is 4 MB
var DISKMAGIC = []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 3, 3, 7} // DISK magic to make sure we are continuing on the right disk

func isDiskMagic(buf []byte) bool {
	n := len(DISKMAGIC)
	if len(buf) < n {
		return false
	}
	for i := 0; i < n; i++ {
		if buf[i] != DISKMAGIC[i] {
			return false
		}
	}
	return true
}

type Disk struct {
	path string   // access path for disk
	size int64    // disk size
	f    *os.File // file handle for disk
}

func CreateDisk(path string) Disk {
	return Disk{path: path}
}

func (d *Disk) Open() error {
	var err error
	if d.f, err = os.OpenFile(d.path, os.O_RDWR, 0640); err != nil {
		d.Close()
		return err
	}
	if d.size, err = d.getDiskSize(); err != nil {
		d.Close()
		return err
	}
	return nil
}
func (d *Disk) Close() error {
	if d.f != nil {
		err := d.f.Close()
		d.f = nil
		return err
	}
	return nil
}

func (d *Disk) seekWhence(whence int) (int64, error) {
	if d.f == nil {
		return 0, fmt.Errorf("disk not opened")
	}
	return d.f.Seek(0, whence)
}

func (d *Disk) Seek(pos int64) error {
	if d.f == nil {
		return fmt.Errorf("disk not opened")
	}
	if n, err := d.f.Seek(pos, 0); err != nil {
		return err
	} else if n != pos {
		fmt.Fprintf(os.Stderr, "Seek failed. Expected %d but got %d\n", pos, n)
		return fmt.Errorf("seek failed")
	}
	return nil
}

// Get current position on disk
func (d *Disk) Position() (int64, error) {
	// First get current position so we can restore that later
	return d.f.Seek(0, 1)
}

func (d *Disk) Size() int64 {
	return d.size
}

func (d *Disk) getDiskSize() (int64, error) {
	// determine size by seeking at the end of the file
	var err error
	var size int64
	var pos int64
	// First get current position so we can restore that later
	if pos, err = d.Position(); err != nil {
		return 0, err
	}
	if size, err = d.f.Seek(0, io.SeekEnd); err != nil {
		return 0, err
	}
	if _, err = d.f.Seek(0, io.SeekStart); err != nil {
		return size, err
	}

	// Restore position, if not at beginning
	if pos != 0 {
		if _, err = d.f.Seek(pos, io.SeekStart); err != nil {
			return size, err
		}
	}

	return size, nil
}

// Check for magic bytes at the beginning of the disk
func (d *Disk) CheckMagic() error {
	if d.f == nil {
		return fmt.Errorf("disk not opened")
	}
	if off, err := d.seekWhence(io.SeekStart); err != nil {
		return err
	} else if off != 0 {
		return fmt.Errorf("seek start failed (pos %d)", off)
	}

	// Read magic bytes at beginning
	buf := make([]byte, CHUNKSIZE)
	if n, err := d.f.Read(buf); err != nil {
		return err
	} else if n != CHUNKSIZE {
		return fmt.Errorf("cannot read full chunk")
	}

	if !isDiskMagic(buf) {
		return fmt.Errorf("invalid disk magic")
	}

	return nil
}

/* Prepare the disk for usage
 * This is already a destructive function as it writes the magic bytes to the beginning of the disk!
 */
func (d *Disk) Prepare() error {
	if d.f == nil {
		return fmt.Errorf("disk not opened")
	}

	if _, err := d.f.Write(DISKMAGIC); err != nil {
		return err
	}
	return d.f.Sync()
}

/* Writes the given chunk
 * Warning: This function does not check if the disk is opened!
 */
func (d *Disk) Write(buf []byte) (int, error) {
	return d.f.Write(buf)
}

/* Reads at the current position
 * Warning: This function does not check if the disk is opened!
 */
func (d *Disk) Read(buf []byte) (int, error) {
	return d.f.Read(buf)
}

/* Performs a sync
 * Warning: This function does not check if the disk is opened!
 */
func (d *Disk) Sync() error {
	return d.f.Sync()
}
