# disko-san

`disko-san` is a simple tool to check the sanity of new hard drives.

The sanity check is done by first writing random data to the disk, which then is read and verified by validating checksum. Data is written as 4 MiB chunks, each sonsisting of a 4 byte checksum plus random data. The checksum allows the program, if the chunk is valid or if the data has been corrupted.

If provided with a STATE file, `disko-san` can stop and resume its operation afterwards. This is useful for large disks, where the host system requires to undergo a system reboot or any other kind of interruption. `disko-san` will be able to resume the process, where it was left before.

In addition, `disko-san` can log write performance metrics to a file. This PERFLOG can be used to check if the write performance of the disk remains stable throughout its capacity. This is useful to check for bad sectors, where the write performance might not be stable.

## Usage

    disko-san DISK [STATE] [PERFLOG]
	
	  DISK          defines the disk under test
	  STATE         progress file, required for resume operations
	  PERFLOG       write performance (write metrics) to this file

`analyse.py` is a small python script to analyse the PERFLOG. It prints the min,max and average values of different subsets of all values (99% values and 68% values)

    ./analyse.py PERFLOG

## Building

`disko-san` is written in plain go without additional requirements

    go build -o disko-san disko-san.go

or the lazy way 

    make

# Disclaimer

The software is provided as-is without any warranty of claims to be correct or even working at all. I'm a random dude from the internet, it's up to you to decide to trust me or not :-)