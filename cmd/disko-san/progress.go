package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
)

// Progress struct for continuing
type Progress struct {
	filename string // Filename of the progress file
	Size     int64  // Disk size
	Pos      int64  // Disk position
	State    int    // State of the process (0 = prepare, 1 = write, 2 = read, 3 = completed)

	f *os.File // Progress file handle or nil, if not present
}

func (p *Progress) Open(filename string) error {
	var err error
	p.f, err = os.OpenFile(filename, os.O_RDWR|os.O_CREATE, 0640)
	if err != nil {
		p.f = nil
		return err
	}
	p.filename = filename
	return nil
}

func (p *Progress) Close() error {
	if p.f != nil {
		err := p.f.Close()
		p.f = nil
		return err
	}
	return nil
}

func (p *Progress) Read() error {
	var err error
	if p.f == nil {
		return fmt.Errorf("no file opened")
	}

	// Seek to beginning of file. This is required as we leave the file opened
	if _, err := p.f.Seek(0, io.SeekStart); err != nil {
		return err
	}

	// Line by line
	scanner := bufio.NewScanner(p.f)
	if !scanner.Scan() {
		return fmt.Errorf("Premature file ending")
	}
	if p.Size, err = strconv.ParseInt(scanner.Text(), 10, 64); err != nil {
		return err
	}
	if !scanner.Scan() {
		return fmt.Errorf("Premature file ending")
	}
	if p.Pos, err = strconv.ParseInt(scanner.Text(), 10, 64); err != nil {
		return err
	}
	if !scanner.Scan() {
		return fmt.Errorf("Premature file ending")
	}
	if p.State, err = strconv.Atoi(scanner.Text()); err != nil {
		return err
	}
	return nil
}

func (p *Progress) Write() error {
	if p.f == nil {
		return fmt.Errorf("no file opened")
	}

	// Seek to beginning of file. This is required as we leave the file opened
	if _, err := p.f.Seek(0, io.SeekStart); err != nil {
		return err
	}

	str := fmt.Sprintf("%d\n%d\n%d", p.Size, p.Pos, p.State)
	if n, err := p.f.Write([]byte(str)); err != nil {
		return err
	} else {
		// Truncate file to the written buffer only
		if _, err := p.f.Seek(0, io.SeekStart); err != nil {
			return err
		}
		return p.f.Truncate(int64(n))
	}
}

func (p *Progress) WriteIfOpen() error {
	if p.f == nil {
		return nil
	}
	return p.Write()
}

func (p *Progress) Sync() error {
	if p.f == nil {
		return fmt.Errorf("no file opened")
	}
	return p.f.Sync()
}
