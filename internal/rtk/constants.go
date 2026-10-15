package rtk

const (
	RawCap                = 10 * 1024 * 1024 // 10 MiB
	MinCompressSize       = 500              // bytes; skip tiny blobs
	DetectWindow          = 1024             // autodetect peeks first N chars
	GitDiffHunkMaxLines   = 100              // per-hunk line cap
	GitDiffMaxLines       = 500              // gitDiff line cap
	GitLogMaxLines        = 200              // gitLog line cap
	DedupLineMax          = 2000             // dedupLog truncation cap
	GrepPerFileMax        = 10               // match rust: matches.iter().take(10)
	FindPerDirMax         = 10               // match rust: files.iter().take(10)
	FindTotalDirMax       = 20               // match rust: dirs.iter().take(20)
	StatusMaxFiles        = 10               // status_max_files
	StatusMaxUntracked    = 10               // status_max_untracked
	LsExtSummaryTop       = 5                // top-N extensions in summary
	TreeMaxLines          = 200              // cap tree output
	SearchListPerDirMax   = 10               // search list per-dir cap
	SearchListTotalDirMax = 20               // search list total dir cap
	SmartTruncateHead     = 120              // lines kept from top
	SmartTruncateTail     = 60               // lines kept from bottom
	SmartTruncateMinLines = 250              // min lines before truncate kicks in
	ReadNumberedMinRatio  = 0.7              // min hit ratio for numbered file dumps
)

var LsNoiseDirs = map[string]bool{
	"node_modules":   true,
	".git":           true,
	"target":         true,
	"__pycache__":    true,
	".next":          true,
	"dist":           true,
	"build":          true,
	".cache":         true,
	".turbo":         true,
	".vercel":        true,
	".pytest_cache":  true,
	".mypy_cache":    true,
	".tox":           true,
	".venv":          true,
	"venv":           true,
	"env":            true,
	"coverage":       true,
	".nyc_output":    true,
	".DS_Store":      true,
	"Thumbs.db":      true,
	".idea":          true,
	".vscode":        true,
	".vs":            true,
	"*.egg-info":     true,
	".eggs":          true,
}
