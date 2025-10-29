package github

// PR holds minimal PR metadata needed by voice flow
type PR struct {
	Number     int    `json:"number"`
	Title      string `json:"title"`
	Author     string `json:"author"`
	Status     string `json:"status"`
	URL        string `json:"url"`
	Repository string `json:"repository"`
}

type Comment struct {
	Author    string `json:"author"`
	Body      string `json:"body"`
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"` // review | inline | general
	Path      string `json:"path,omitempty"`
	Line      int    `json:"line,omitempty"`
}

type Status struct {
	ChecksPassing   int      `json:"checksPassing"`
	ChecksTotal     int      `json:"checksTotal"`
	Approvals       []string `json:"approvals"`
	Mergeable       bool     `json:"mergeable"`
	HasConflicts    bool     `json:"hasConflicts"`
	FailingCheckIDs []string `json:"failingCheckIds,omitempty"`
}

type Diff struct {
	FilesChanged int        `json:"filesChanged"`
	Additions    int        `json:"additions"`
	Deletions    int        `json:"deletions"`
	Files        []DiffFile `json:"files"`
}

type DiffFile struct {
	Filename  string `json:"filename"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	Patch     string `json:"patch,omitempty"`
}
