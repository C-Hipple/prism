package types

// LineComment defines the structure for a single line-specific review comment.
type LineComment struct {
	FilePath    string `json:"file_path"`
	LineNumber  int    `json:"line_number"`
	CommentBody string `json:"comment_body"`
	Importance  string `json:"importance,omitempty"`
}
