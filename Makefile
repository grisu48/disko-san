default: all
all: disko-san

PREFIX=/usr/local/bin
GOARGS=

disko-san: cmd/disko-san/disko-san.go cmd/disko-san/chunk.go cmd/disko-san/disk.go cmd/disko-san/progress.go
	go build $(GOARGS) -o $@ $^

install: disko-san
	install disko-san $(PREFIX)
