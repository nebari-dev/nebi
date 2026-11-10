package main

import "testing"

func TestFormatDownloadSize(t *testing.T) {
	cases := map[int64]string{
		0:          "0 B",
		512:        "512 B",
		125_000:    "125 KB",
		69_700:     "69.7 KB",
		1_700_000:  "1.7 MB",
		21_600_000: "21.6 MB",
	}
	for in, want := range cases {
		if got := formatDownloadSize(in); got != want {
			t.Errorf("formatDownloadSize(%d) = %q, want %q", in, got, want)
		}
	}
}
