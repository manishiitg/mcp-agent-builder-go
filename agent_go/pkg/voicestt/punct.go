package voicestt

// isASCIIText reports whether every byte is 7-bit ASCII — the cheap test for
// "this is English the punctuation model was trained on", since any other
// script arrives as multi-byte UTF-8.
func isASCIIText(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}
