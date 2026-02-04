package db

import (
	"time"
)

// PR represents a pull request in the database
type PR struct {
	ID              int
	RepoOwner       string
	RepoName        string
	PRNumber        int
	LastCommitSHA   string
	LastReviewedAt  *time.Time
	ReviewHTMLPath  string
	Status          string // "pending", "generating", "completed", "error"
	GeneratingSince *time.Time
	Title           string     // PR title from GitHub
	Author          string     // PR author from GitHub
	ApprovalCount   int        // Number of current approvals
	MyReviewStatus  string     // Current user's review status: "APPROVED", "CHANGES_REQUESTED", "COMMENTED", or ""
	CreatedAt       *time.Time // PR creation timestamp from GitHub
	Draft           bool       // true if PR is in draft mode
	CIState         string     // CI status: "success", "failure", "pending", "unknown"
	CIFailedChecks  string     // JSON array of failed check names
	// Review importance counts
	CriticalCount int // Number of CRITICAL importance comments
	MediumCount   int // Number of MEDIUM importance comments
	LowCount      int // Number of LOW importance comments
	// User notes (single-user mode)
	Notes string
}

// User represents a user in multi-user mode
type User struct {
	ID              int
	GitHubID        int64
	GitHubUsername  string
	GitHubAvatarURL string
	CreatedAt       time.Time
	LastLoginAt     *time.Time
}

// Session represents a user session
type Session struct {
	ID        string
	UserID    int
	ExpiresAt time.Time
	CreatedAt time.Time
}

// UserPRAssignment represents the relationship between users and PRs
// Deprecated: Use UserPRView instead. This type is kept for backward compatibility.
type UserPRAssignment struct {
	ID             int
	UserID         int
	PRID           int
	IsAuthor       bool
	IsReviewer     bool
	ReviewerGroups string // JSON array of team names (deprecated, use ViaTeams)
	MyReviewStatus string // User's review status for this PR
	Notes          string // User's notes for this PR
}

// UserPRView represents the relationship between users and PRs (new name for UserPRAssignment)
// This is the preferred type for new code.
type UserPRView struct {
	ID           int
	UserID       int
	PRID         int
	IsAuthor     bool
	IsReviewer   bool
	ViaTeams     string // JSON array of team names (was ReviewerGroups)
	ReviewStatus string // User's review status for this PR (was MyReviewStatus)
	Notes        string // User's notes for this PR
	Hidden       bool   // Whether this PR is hidden from the user's view
}

// PRWithUserView combines PR data with user-specific view data
type PRWithUserView struct {
	PR
	IsAuthor     bool   // From user_pr_views
	IsReviewer   bool   // From user_pr_views
	UserNotes    string // Notes from user_pr_views (overrides PR.Notes)
	ReviewStatus string // User's review status from user_pr_views
}

// Database defines the interface that both SQLite and PostgreSQL implementations must satisfy
type Database interface {
	// PR operations
	GetPR(owner, repo string, prNumber int) (*PR, error)
	UpsertPR(pr *PR) error
	UpdatePRStatus(owner, repo string, prNumber int, status string) error
	ResetPRToOutdated(owner, repo string, prNumber int, newCommitSHA string) error
	SetPRGenerating(owner, repo string, prNumber int, commitSHA, title, author string, createdAt *time.Time, draft bool) error
	GetAllPRs() ([]PR, error)
	DeletePR(owner, repo string, prNumber int) error
	ResetStaleGeneratingPRs(timeoutMinutes int) (int, error)
	ResetErrorPRs(maxAgeMinutes int) (int, error)
	GetPRsWithMissingMetadata() ([]PR, error)
	UpdatePRMetadata(owner, repo string, prNumber int, title, author string) error
	UpdatePRNotes(owner, repo string, prNumber int, notes string) error
	GetPRsWithMissingCreatedAt() ([]PR, error)
	UpdatePRCreatedAt(owner, repo string, prNumber int, createdAt time.Time) error

	// Settings operations
	GetSetting(key string) (string, error)
	SetSetting(key, value string) error
	GetAutoReviewRequestedPRs() (bool, error)
	SetAutoReviewRequestedPRs(enabled bool) error
	GetReviewNRequests() (int, error)
	SetReviewNRequests(n int) error
	GetGenerateHTML() (bool, error)
	SetGenerateHTML(enabled bool) error

	// User operations
	GetUserByGitHubID(githubID int64) (*User, error)
	GetUserByID(id int) (*User, error)
	GetUserByUsername(username string) (*User, error)
	CreateUser(user *User) error
	UpdateUserLastLogin(userID int) error

	// Session operations (multi-user mode only)
	CreateSession(session *Session) error
	GetSession(id string) (*Session, error)
	DeleteSession(id string) error

	// User-PR view operations
	GetUserPRAssignment(userID, prID int) (*UserPRAssignment, error)
	UpsertUserPRAssignment(assignment *UserPRAssignment) error
	GetPRsForUser(userID int) ([]PR, error)
	GetPRsForUserWithNotes(userID int) ([]PRWithUserView, error)
	UpdateUserPRNotes(userID, prID int, notes string) error
	HidePRForUser(userID, prID int) error
	EnsureUserPRView(userID, prID int, isAuthor bool) error
	MigrateLegacyNotes(userID int) (int, error)

	// Lifecycle
	Close() error
}
