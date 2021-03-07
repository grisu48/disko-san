# disko-san

`disko-san` is a simple tool to check the sanity of new hard drives. It check the sanity by first writing to the disk and then verifying, if the written data is OK. Data is written in form of random chunks, each 4 MB in size. Chunks consists of a chunk checksum together with random data. The checksum allows the program to verify, if the chunk is OK.

`disko-san` first writes the disk full with random chunks. This should test if there are any bad sectors. Performance metrics are can be written to a PERFLOG file. It then reads all written chunks and verifies them according to the written checksum. This allows to check, if the disk is healthy throughout its whole capacity. Because it first write the FULL disk and then read it from scratch, the disk cache should not be used, so that we are testing the actual physical disk write-read operation.
After a successfull test run, the PERFLOG can be used to check if the performance throughout the disk remains the same.

If provided with a STATE file, `disko-san` can break and resume its operation afterwards. This is useful for large disks, where the host system requires to undergo a system reboot or another kind of break without restarting the whole process again.


## Usage

    disko-san DISK [STATE] [PERFLOG]
	
	  DISK          defines the disk under test
	  STATE         progress file, required for resume operations
	  PERFLOG       write performance (write metrics) to this file

## Building

`disko-san` is written in plain go without additional requirements

    go build -o disko-san disko-san.go

or the lazy way 

    make

