.PHONY all: out/client-linux-amd64 out/client-windows-amd64.exe out/server

out/client-linux-amd64: out cmd/client
	CGO_ENABLED=0 go build -o $@ edu/cmd/client

out/client-windows-amd64.exe: out cmd/client
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o $@ edu/cmd/client

out/server: out cmd/server
	CGO_ENABLED=0 GOARCH=386 go build -o $@ edu/cmd/server
	go run encode.go $@

out:
	mkdir -p $@
