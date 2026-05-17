package model

import (
	"testing"
	"time"
)

func TestArticleIsRead(t *testing.T) {
	loc := time.Local
	today := time.Date(2026, 5, 17, 10, 0, 0, 0, loc)
	yesterday := time.Date(2026, 5, 16, 23, 30, 0, 0, loc)
	todayMidnight := time.Date(2026, 5, 17, 0, 0, 0, 0, loc)

	tests := []struct {
		name      string
		readAt    *time.Time
		ephemeral bool
		now       time.Time
		want      bool
	}{
		{"nil read_at regular", nil, false, today, false},
		{"nil read_at ephemeral", nil, true, today, false},
		{"set read_at regular", &yesterday, false, today, true},
		{"ephemeral read today", &today, true, today, true},
		{"ephemeral read yesterday", &yesterday, true, today, false},
		{"ephemeral read at today midnight", &todayMidnight, true, today, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ArticleIsRead(Article{ReadAt: tc.readAt}, tc.ephemeral, tc.now)
			if got != tc.want {
				t.Fatalf("ArticleIsRead(readAt=%v, ephemeral=%v, now=%v) = %v, want %v",
					tc.readAt, tc.ephemeral, tc.now, got, tc.want)
			}
		})
	}
}
