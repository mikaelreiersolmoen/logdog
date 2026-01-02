package logcat

import "testing"

func TestParseLinePreservesLeadingIndentation(t *testing.T) {
	line := "12-14 15:31:12.345  1234  5678 D MyTag:     Indented message"

	entry, err := ParseLine(line)
	if err != nil {
		t.Fatalf("ParseLine returned error: %v", err)
	}

	want := "    Indented message"
	if entry.Message != want {
		t.Fatalf("expected message %q, got %q", want, entry.Message)
	}
}

func TestParseLineTrimsLogcatPaddingOnly(t *testing.T) {
	line := "12-14 15:31:12.345  1234  5678 D MyTag: Normal message"

	entry, err := ParseLine(line)
	if err != nil {
		t.Fatalf("ParseLine returned error: %v", err)
	}

	want := "Normal message"
	if entry.Message != want {
		t.Fatalf("expected message %q, got %q", want, entry.Message)
	}
}
