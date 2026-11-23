package service

import (
	"testing"
	"time"
)

func TestPhotoDateFormat(t *testing.T) {
	taken := time.Date(2019, time.July, 4, 15, 30, 0, 0, time.UTC)
	cases := []struct {
		format string
		want   string
	}{
		{"", "Jul 04, 2019"}, // the default keeps the year
		{"02.01.2006", "04.07.2019"},
		{"2006-01-02", "2019-07-04"},
		{"Monday, January 02, 2006", "Thursday, July 04, 2019"},
	}
	for _, tc := range cases {
		if got := taken.Format(photoDateFormat(tc.format)); got != tc.want {
			t.Errorf("format %q: got %q, want %q", tc.format, got, tc.want)
		}
	}
	// Today's date keeps its own default.
	if dateFormat("") != "Mon, Jan 02" {
		t.Errorf("dateFormat default changed: %q", dateFormat(""))
	}
}
