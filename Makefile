.PHONY all: out/client out/client-windows.exe out/server

out/client: out cmd/client
	CGO_ENABLED=0 go build -o $@ edu/cmd/client

out/client-windows.exe: out cmd/client
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o $@ edu/cmd/client

out/server: out cmd/server
	CGO_ENABLED=0 GOARCH=386 go build -o $@ edu/cmd/server
	cd $(@D) && tar cJf - $(@F) | base64 -w 300 >$(@F).txz.b64.txt

out:
	mkdir -p $@
