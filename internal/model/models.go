package model

import (
	"time"
)

type Role string

const (
	RoleAdmin        Role = "admin"
	RoleInstAdmin    Role = "inst_admin"
	RoleProofreader1 Role = "proofreader1"
	RoleProofreader2 Role = "proofreader2"
	RoleTypesetter   Role = "typesetter"
)

type UserStatus string

const (
	UserStatusActive  UserStatus = "active"
	UserStatusPending UserStatus = "pending"
	UserStatusLocked  UserStatus = "locked"
)

type User struct {
	ID               uint64         `gorm:"primaryKey;autoIncrement" json:"id"`
	Email            string         `gorm:"size:255;uniqueIndex;not null" json:"email"`
	PasswordHash     string         `gorm:"size:255;not null" json:"-"`
	Name             string         `gorm:"size:100;not null" json:"name"`
	Role             Role           `gorm:"size:20;not null" json:"role"`
	InstitutionID    *uint64        `gorm:"index" json:"institution_id,omitempty"`
	TOTPSecret       string         `gorm:"size:255" json:"-"`
	TOTPEnabled      bool           `gorm:"default:false" json:"totp_enabled"`
	Status           UserStatus     `gorm:"size:20;default:'pending'" json:"status"`
	LastPasswordChange time.Time    `json:"last_password_change"`
	PasswordExpired  bool           `gorm:"default:false" json:"password_expired"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
}

type InstitutionType string

const (
	InstTypeUniversity InstitutionType = "university"
	InstTypePress      InstitutionType = "press"
	InstTypeLocalGov   InstitutionType = "local_government"
	InstTypeOther      InstitutionType = "other"
)

type InstitutionStatus string

const (
	InstStatusPending  InstitutionStatus = "pending"
	InstStatusApproved InstitutionStatus = "approved"
	InstStatusRejected InstitutionStatus = "rejected"
)

type Institution struct {
	ID                   uint64            `gorm:"primaryKey;autoIncrement" json:"id"`
	Name                 string            `gorm:"size:255;not null" json:"name"`
	Type                 InstitutionType   `gorm:"size:30;not null" json:"type"`
	Status               InstitutionStatus `gorm:"size:20;default:'pending'" json:"status"`
	AdminID              *uint64           `gorm:"index" json:"admin_id,omitempty"`
	PasswordExpiryDays   int               `gorm:"default:90" json:"password_expiry_days"`
	PasswordChangeForced bool              `gorm:"default:true" json:"password_change_forced"`
	TOTPRequired         bool              `gorm:"default:false" json:"totp_required"`
	CreatedAt            time.Time         `json:"created_at"`
	UpdatedAt            time.Time         `json:"updated_at"`
}

type ProjectStatus string

const (
	ProjectStatusPending   ProjectStatus = "pending"
	ProjectStatusActive    ProjectStatus = "active"
	ProjectStatusCompleted ProjectStatus = "completed"
	ProjectStatusArchived  ProjectStatus = "archived"
)

type Project struct {
	ID             uint64        `gorm:"primaryKey;autoIncrement" json:"id"`
	InstitutionID  uint64        `gorm:"not null;index" json:"institution_id"`
	Title          string        `gorm:"size:500;not null" json:"title"`
	Author         string        `gorm:"size:255" json:"author"`
	VersionInfo    string        `gorm:"size:500" json:"version_info"`
	StartPage      int           `gorm:"default:1" json:"start_page"`
	EndPage        int           `json:"end_page"`
	PageCount      int           `gorm:"default:0" json:"page_count"`
	Status         ProjectStatus `gorm:"size:20;default:'pending'" json:"status"`
	CreatedBy      uint64        `gorm:"not null" json:"created_by"`
	ReviewRequired int           `gorm:"default:1" json:"review_required"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
}

type PageStatus string

const (
	PageStatusUnassigned PageStatus = "unassigned"
	PageStatusAssigned   PageStatus = "assigned"
	PageStatusProofing   PageStatus = "proofing"
	PageStatusReviewing  PageStatus = "reviewing"
	PageStatusCompleted  PageStatus = "completed"
	PageStatusRejected   PageStatus = "rejected"
)

type Page struct {
	ID            uint64     `gorm:"primaryKey;autoIncrement" json:"id"`
	ProjectID     uint64     `gorm:"not null;index:idx_project_page,unique" json:"project_id"`
	PageNumber    int        `gorm:"not null;index:idx_project_page,unique" json:"page_number"`
	ImagePath     string     `gorm:"size:500" json:"image_path"`
	TilePath      string     `gorm:"size:500" json:"tile_path"`
	OCRText       string     `gorm:"type:text" json:"ocr_text"`
	CorrectedText string     `gorm:"type:text" json:"corrected_text"`
	LinePositions string     `gorm:"type:json" json:"line_positions,omitempty"`
	AssignedTo    *uint64    `gorm:"index" json:"assigned_to,omitempty"`
	Status        PageStatus `gorm:"size:20;default:'unassigned'" json:"status"`
	Version       string     `gorm:"size:100;default:'main'" json:"version"`
	VersionLabel  string     `gorm:"size:100" json:"version_label"`
	LockBy        *uint64    `gorm:"index" json:"lock_by,omitempty"`
	LockAt        *time.Time `json:"lock_at,omitempty"`
	AutoSavedAt   *time.Time `json:"auto_saved_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type CorrectionType string

const (
	CorrectionTypeWrong    CorrectionType = "wrong"
	CorrectionTypeMissing  CorrectionType = "missing"
	CorrectionTypeExtra    CorrectionType = "extra"
	CorrectionTypeReversed CorrectionType = "reversed"
	CorrectionTypeVariant  CorrectionType = "variant"
)

type Correction struct {
	ID            uint64         `gorm:"primaryKey;autoIncrement" json:"id"`
	PageID        uint64         `gorm:"not null;index" json:"page_id"`
	Type          CorrectionType `gorm:"size:20;not null" json:"type"`
	StartPosition int            `gorm:"not null" json:"start_position"`
	EndPosition   int            `json:"end_position"`
	OriginalText  string         `gorm:"type:text" json:"original_text"`
	CorrectedText string         `gorm:"type:text" json:"corrected_text"`
	Note          string         `gorm:"type:text" json:"note"`
	Emendation    string         `gorm:"type:text" json:"emendation"`
	VariantID     *uint64        `gorm:"index" json:"variant_id,omitempty"`
	IsVariantAuto bool           `gorm:"default:false" json:"is_variant_auto"`
	CreatedBy     uint64         `gorm:"not null" json:"created_by"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

type VariantChar struct {
	ID           uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	VariantChar  string    `gorm:"size:10;not null;index" json:"variant_char"`
	StandardChar string    `gorm:"size:10;not null" json:"standard_char"`
	Source       string    `gorm:"size:255" json:"source"`
	Verified     bool      `gorm:"default:false" json:"verified"`
	Frequency    int       `gorm:"default:0" json:"frequency"`
	CreatedAt    time.Time `json:"created_at"`
}

type CustomVariant struct {
	ID           uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	ProjectID    uint64    `gorm:"not null;index:idx_project_variant,unique" json:"project_id"`
	VariantChar  string    `gorm:"size:10;not null;index:idx_project_variant,unique" json:"variant_char"`
	StandardChar string    `gorm:"size:10;not null" json:"standard_char"`
	CreatedBy    uint64    `gorm:"not null" json:"created_by"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type VersionCompareTask struct {
	ID               uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	ProjectID        uint64    `gorm:"not null;index" json:"project_id"`
	VersionAPageID   uint64    `gorm:"not null" json:"version_a_page_id"`
	VersionBPageID   uint64    `gorm:"not null" json:"version_b_page_id"`
	VersionALabel    string    `gorm:"size:100" json:"version_a_label"`
	VersionBLabel    string    `gorm:"size:100" json:"version_b_label"`
	DiffResultJSON   string    `gorm:"type:longtext" json:"diff_result_json"`
	EmendationText   string    `gorm:"type:text" json:"emendation_text"`
	CreatedBy        uint64    `gorm:"not null" json:"created_by"`
	CreatedAt        time.Time `json:"created_at"`
}

type ReviewStatus string

const (
	ReviewStatusPending  ReviewStatus = "pending"
	ReviewStatusApproved ReviewStatus = "approved"
	ReviewStatusRejected ReviewStatus = "rejected"
)

type ReviewRound struct {
	ID        uint64       `gorm:"primaryKey;autoIncrement" json:"id"`
	PageID    uint64       `gorm:"not null;index" json:"page_id"`
	ReviewerID uint64      `gorm:"not null" json:"reviewer_id"`
	RoundNum  int          `gorm:"default:1" json:"round_num"`
	Status    ReviewStatus `gorm:"size:20;default:'pending'" json:"status"`
	Comment   string       `gorm:"type:text" json:"comment"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
}

type AuditLog struct {
	ID         uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID     uint64    `gorm:"index" json:"user_id"`
	Action     string    `gorm:"size:100;not null;index" json:"action"`
	TargetType string    `gorm:"size:50;not null" json:"target_type"`
	TargetID   uint64    `gorm:"not null" json:"target_id"`
	IPAddress  string    `gorm:"size:50" json:"ip_address"`
	Details    string    `gorm:"type:text" json:"details"`
	CreatedAt  time.Time `gorm:"not null;index" json:"created_at"`
}

type Session struct {
	ID           uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID       uint64    `gorm:"not null;index" json:"user_id"`
	RefreshToken string    `gorm:"size:500;not null;uniqueIndex" json:"-"`
	UserAgent    string    `gorm:"size:500" json:"user_agent"`
	IPAddress    string    `gorm:"size:50" json:"ip_address"`
	ExpiresAt    time.Time `gorm:"not null;index" json:"expires_at"`
	IsActive     bool      `gorm:"default:true" json:"is_active"`
	CreatedAt    time.Time `json:"created_at"`
}
