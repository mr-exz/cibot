package telegram

import "time"

// FlowType represents the type of issue creation or admin flow
type FlowType string

const (
	FlowSupport FlowType = "support" // self-service /support flow
	FlowTicket  FlowType = "ticket"  // support-assisted /ticket flow
	FlowAdmin   FlowType = "admin"   // admin configuration flow
)

// AdminCmd represents a specific admin command
type AdminCmd string

const (
	AdminCmdAddCategory  AdminCmd = "addcategory"
	AdminCmdAddType      AdminCmd = "addtype"
	AdminCmdAddPerson    AdminCmd = "addperson"
	AdminCmdSetRotation  AdminCmd = "setrotation"
	AdminCmdSetWorkHours AdminCmd = "setworkhours"
	AdminCmdAddTopic     AdminCmd = "addtopic"
)

// Step constants for the multi-step issue creation flow
const (
	StepCategory    = "category"
	StepRequestType = "request_type"
	StepTitle       = "title"
	StepDescription = "description"
)

// Admin flow steps
const (
	// addcategory steps
	StepAdminCatName    = "admin_cat_name"
	StepAdminCatEmoji   = "admin_cat_emoji"
	StepAdminCatTeamKey = "admin_cat_teamkey"

	// addtype steps
	StepAdminTypeName = "admin_type_name"

	// addperson steps
	StepAdminPersonName     = "admin_person_name"
	StepAdminPersonTelegram = "admin_person_tg"
	StepAdminPersonLinear   = "admin_person_linear"
	StepAdminPersonTimezone = "admin_person_tz"
	StepAdminPersonHours    = "admin_person_hours"
	StepAdminPersonDays     = "admin_person_days"

	// setworkhours steps
	StepAdminSelectPerson = "admin_select_person"
	StepAdminWhTimezone   = "admin_wh_timezone"
	StepAdminWhHours      = "admin_wh_hours"
	StepAdminWhDays       = "admin_wh_days"

	// setrotation steps
	StepAdminSelectRotationType = "admin_select_rotation_type"

	// addtopic steps
	StepAdminTopicSelectGroup = "admin_topic_select_group"
	StepAdminTopicName        = "admin_topic_name"
	StepAdminTopicID          = "admin_topic_id"

	// addcategory manual topic steps (DM flow)
	StepAdminCatManualGroup   = "admin_cat_manual_group"
	StepAdminCatManualTopicID = "admin_topic_manual_id"
)

// pendingSession represents an in-progress issue creation session for a user
type pendingSession struct {
	Flow         FlowType
	Step         string
	UserID       int64
	CreatedAt    time.Time
	CategoryID   int64
	CategoryName string
	TeamKey      string
	TypeID       int64
	TypeName     string
	Title        string
	MediaLinks   []string
	MessageID    int
	ChatID       int64
	ThreadID     int

	// ticket-specific fields
	TicketMsgLink    string
	TicketMsgBody    string
	TicketMsgDate    time.Time
	ReporterUsername string
	ReporterName     string
}

// pendingAdminSession represents an in-progress admin configuration session
type pendingAdminSession struct {
	Cmd       AdminCmd
	Step      string
	MessageID int
	ChatID    int64
	CreatedAt time.Time
	ThreadID  int   // captured from originating message for topic-aware addcategory
	UserID    int64 // admin user ID for state management

	// Accumulated data
	CategoryID     int64
	CategoryName   string
	TeamKey        string // Linear team key
	TypeName       string
	PersonID       int64
	PersonName     string
	TgUsername     string
	LinearUsername string
	Timezone       string
	WorkHours      string
	WorkDays       string
	RotationType   string

	// addtopic fields
	SelectedChatID int64  // group chat selected in addtopic flow
	TopicName      string // topic name for addtopic flow

	// addcategory DM flow: the group the category should belong to
	TargetGroupChatID int64
}
