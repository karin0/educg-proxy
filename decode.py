#!/usr/bin/env python2
import base64
import gzip
import sys

input_file = sys.argv[1]

with open(input_file) as fi, open(input_file + '.gz', 'wb') as fo:
	s = fi.read().replace('\n', '').replace('\r', '')
	s = base64.b64decode(s)
	fo.write(s)

with gzip.open(input_file + '.gz', "rb") as fi, open(input_file + '.out', "wb") as fo:
	fo.write(fi.read())
