package text

import "testing"

func TestStripHTML(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "plain text passthrough",
			input: "Hello, world!",
			want:  "Hello, world!",
		},
		{
			name:  "html tags stripped",
			input: "<p>Hello <b>world</b>!</p>",
			want:  "Hello world!",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "html entities decoded",
			input: "<p>AT&amp;T &lt;rocks&gt;</p>",
			want:  "AT&T <rocks>",
		},
		{
			name:  "br tags become newlines",
			input: "line1<br>line2<br/>line3",
			want:  "line1\nline2\nline3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripHTML(tt.input)
			if got != tt.want {
				t.Errorf("StripHTML(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
