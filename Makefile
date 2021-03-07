default: disko-san

PREFIX=/usr/local/bin

disko-san: cmd/disko-san/disko-san.go cmd/disko-san/chunk.go cmd/disko-san/disk.go cmd/disko-san/progress.go
	go build -o $@ $^

install: disko-san
	install disko-san $(PREFIX)
