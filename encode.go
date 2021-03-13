package main

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"io/ioutil"
	"os"
)

func panicIf(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {
	bs, err := ioutil.ReadFile(os.Args[1])
	panicIf(err)

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	_, err = gz.Write(bs)
	panicIf(err)
	panicIf(gz.Flush())
	panicIf(gz.Close())

	b64 := base64.StdEncoding.EncodeToString(buf.Bytes())
	fp, err := os.Create(os.Args[1] + "_gz_b64.txt")
	panicIf(err)
	defer fp.Close()
	for i := 0; i < len(b64); {
		end := i + 256
		if end > len(b64) {
			end = len(b64)
		}
		_, err := fp.WriteString(b64[i: end] + "\n")
		panicIf(err)
		i = end
	}
	panicIf(fp.Sync())
}
