#!/usr/bin/env python3

# Copyright 2017 Luke Granger-Brown
# 
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
# 
#      http://www.apache.org/licenses/LICENSE-2.0
# 
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

import hashlib
import struct
import zlib

with open('badmagic.blte', 'wb') as f:
    f.write(b'XLTE\0\0\0\0boo')

with open('noheader.uncompressed.blte', 'wb') as f:
    f.write(b'BLTE\0\0\0\0')
    f.write(b'N')
    f.write(b'this BLTE file contains uncompressed data, with no chunks')

with open('noheader.zlib.blte', 'wb') as f:
    f.write(b'BLTE\0\0\0\0')
    f.write(b'Z')
    f.write(zlib.compress(b'this BLTE file contains zlib-compressed data, with no chunks'))

with open('onechunk.uncompressed.blte', 'wb') as f:
    content = b'this BLTE file contains uncompressed data, with a single chunk'
    cie = struct.pack('>II', len(content)+1, len(content)) + hashlib.md5(b'N' + content).digest()
    ci = struct.pack('>HH', 0x0, 0x1) + cie

    f.write(b'BLTE' + struct.pack('>I', len(ci) + 0x8))
    f.write(ci)
    f.write(b'N')
    f.write(content)

with open('onechunk.zlib.blte', 'wb') as f:
    content = b'this BLTE file contains zlib-compressed data, with a single chunk'
    compressed_content = b'Z' + zlib.compress(content)
    cie = struct.pack('>II', len(compressed_content), len(content)) + hashlib.md5(compressed_content).digest()
    ci = struct.pack('>HH', 0x0, 0x1) + cie

    f.write(b'BLTE' + struct.pack('>I', len(ci) + 0x8))
    f.write(ci)
    f.write(compressed_content)

with open('manychunks.uncompressed.blte', 'wb') as f:
    content = b'this BLTE file contains an obscene number of uncompressed chunks - at least, a sufficient number of chunks to make sure that decoding is happening correctly, even where the number of chunks exceeds 255, since it almost certainly will at some point, and thus we should be prepared.'
    ci = struct.pack('>HH', 0x0, len(content))
    fc = b''
    for c in content:
        compressed_content = b'N' + bytes([c])
        ci += struct.pack('>II', len(compressed_content), len(content)) + hashlib.md5(compressed_content).digest()
        fc += compressed_content
    f.write(b'BLTE' + struct.pack('>I', len(ci) + 0x8))
    f.write(ci)
    f.write(fc)

with open('manychunks.zlib.blte', 'wb') as f:
    content = b'this BLTE file contains an obscene number of zlib-compressed chunks - at least, a sufficient number of chunks to make sure that decoding is happening correctly, even where the number of chunks exceeds 255, since it almost certainly will at some point, and thus we should be prepared.'
    ci = struct.pack('>HH', 0x0, len(content))
    fc = b''
    for c in content:
        compressed_content = b'Z' + zlib.compress(bytes([c]))
        ci += struct.pack('>II', len(compressed_content), len(content)) + hashlib.md5(compressed_content).digest()
        fc += compressed_content
    f.write(b'BLTE' + struct.pack('>I', len(ci) + 0x8))
    f.write(ci)
    f.write(fc)

with open('manychunks.mixed.blte', 'wb') as f:
    content = b'this BLTE file contains an obscene number of a mixture of uncompressed and zlib-compressed chunks - at least, a sufficient number of chunks to make sure that decoding is happening correctly, even where the number of chunks exceeds 255, since it almost certainly will at some point, and thus we should be prepared.'
    ci = struct.pack('>HH', 0x0, len(content))
    fc = b''
    for n, c in enumerate(content):
        compressed_content = bytes([c])
        if n % 2 == 0:
            compressed_content = b'N' + compressed_content
        else:
            compressed_content = b'Z' + zlib.compress(compressed_content)
        ci += struct.pack('>II', len(compressed_content), len(content)) + hashlib.md5(compressed_content).digest()
        fc += compressed_content
    f.write(b'BLTE' + struct.pack('>I', len(ci) + 0x8))
    f.write(ci)
    f.write(fc)
