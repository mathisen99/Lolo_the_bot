package handler

import "testing"

func TestContainsMention(t *testing.T) {
	handler := &MentionHandler{botNick: "Lolo"}

	tests := []struct {
		name    string
		message string
		want    bool
	}{
		{
			name:    "matches plain nick",
			message: "Lolo: hello there",
			want:    true,
		},
		{
			name:    "matches nick case-insensitively",
			message: "lOlO help me",
			want:    true,
		},
		{
			name:    "matches bridge nick exception",
			message: "Lolo/libera: generate examples of c code",
			want:    true,
		},
		{
			name:    "matches bridge nick case-insensitively",
			message: "lolo/LIBERA, test",
			want:    true,
		},
		{
			name:    "does not match other network suffixes",
			message: "Lolo/othernet: test",
			want:    false,
		},
		{
			name:    "does not match nick prefix only",
			message: "Lolo123 can you help?",
			want:    false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := handler.ContainsMention(tc.message)
			if got != tc.want {
				t.Fatalf("ContainsMention(%q) = %v, want %v", tc.message, got, tc.want)
			}
		})
	}
}
