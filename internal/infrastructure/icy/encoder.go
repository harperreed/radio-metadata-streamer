// ABOUTME: ICY metadata block encoding for Shoutcast/Icecast protocol
// ABOUTME: Handles 16-byte padding and length byte calculation per ICY spec
package icy

import "bytes"

// BuildBlock encodes text as ICY metadata block with 16-byte padding.
// Returns length byte (count of 16-byte chunks) followed by padded payload.
// Max size: 255 * 16 = 4080 bytes.
func BuildBlock(text string) []byte {
	if text == "" {
		return []byte{0x00}
	}

	payload := []byte(text)

	// Truncate if exceeds max (255 blocks * 16 bytes)
	if len(payload) > 255*16 {
		payload = payload[:255*16]
	}

	// Calculate blocks (round up)
	blocks := (len(payload) + 15) / 16
	if blocks > 255 {
		blocks = 255
	}

	// Calculate padding
	pad := blocks*16 - len(payload)

	// Build result: length byte + payload + padding
	var buf bytes.Buffer
	buf.WriteByte(byte(blocks))
	buf.Write(payload)
	if pad > 0 {
		buf.Write(bytes.Repeat([]byte{0x00}, pad))
	}

	return buf.Bytes()
}
