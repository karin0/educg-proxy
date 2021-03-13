.PHONY all: out/client-linux-64 out/client-windows-64.exe out/server

out/client-linux-64: out cmd/client
	go build -o $@ edu/cmd/client

out/client-windows-64.exe: out cmd/client
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o $@ edu/cmd/client

out/server: out cmd/server
	GOARCH=386 go build -o $@ edu/cmd/server
	go run encode.go $@

out:
	mkdir -p $@
