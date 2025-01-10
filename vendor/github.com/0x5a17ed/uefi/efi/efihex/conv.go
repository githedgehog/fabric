// Copyright (c) 2022 Arthur Skowronek <0x5a17ed@tuta.io> and contributors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// <https://www.apache.org/licenses/LICENSE-2.0>
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package efihex

const hexTable = "0123456789ABCDEF"

func EncodeBigEndian(dst, src []byte) {
	if len(src) <= 0 {
		return
	}

	_ = dst[len(src)*2-1] // bounds check hint to compiler
	for i, j := 0, 0; i < len(src); i, j = i+1, j+2 {
		dst[j], dst[j+1] = hexTable[src[i]>>4], hexTable[src[i]&0x0f]
	}
}

func EncodeLittleEndian(dst, src []byte) {
	_ = dst[len(src)*2-1] // bounds check hint to compiler
	for i, j := 0, 0; i < len(src); i, j = i+1, j+2 {
		dst[j], dst[j+1] = hexTable[src[len(src)-i-1]>>4], hexTable[src[len(src)-i-1]&0x0f]
	}
}
