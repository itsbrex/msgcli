package cmd

import "testing"

func TestShortMessageID(t *testing.T) {
	full := "AAMkADc0MTkxN2M4LWZmZjMtNDZjYS1hYWYyLWI5YmUxMDhiZjJjNQBGAAAAAACujF85aoo0T7DjYTysAMKFBwBbygBSv_9SQIjLrvIJpLyiAAAAAAEMAABbygBSv_9SQIjLrvIJpLyiAAB5sa9_AAA="
	short := shortMessageID(full)
	if len(short) >= len(full) {
		t.Fatalf("expected shortened message ID")
	}
	if short[:14] != full[:14] {
		t.Fatalf("expected head of short ID to match original")
	}
	if short[len(short)-12:] != full[len(full)-12:] {
		t.Fatalf("expected tail of short ID to match original")
	}
}

func TestTruncateText(t *testing.T) {
	if got := truncateText("abc", 10); got != "abc" {
		t.Fatalf("unexpected non-truncated text: %q", got)
	}
	if got := truncateText("abcdefghijklmnopqrstuvwxyz", 10); got != "abcdefg..." {
		t.Fatalf("unexpected truncated text: %q", got)
	}
}
