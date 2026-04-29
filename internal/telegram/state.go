package telegram

import "time"

// FlowType represents the type of issue creation or admin flow
type FlowType string

const (
	FlowSupport      FlowType = "support"       // self-service /ticket flow
	FlowTicket       FlowType = "ticket"        // reply-based /ticket flow
	FlowAdmin        FlowType = "admin"         // admin configuration flow
	FlowUpdateLinear FlowType = "update_linear" // /mylinear account update
	FlowThread       FlowType = "thread"        // /thread: create Linear issue + tech topic
)

// AdminCmd represents a specific admin command
type AdminCmd string

const (
	AdminCmdAddCategory         AdminCmd = "addcategory"
	AdminCmdAddType             AdminCmd = "addtype"
	AdminCmdAddPerson           AdminCmd = "addperson"
	AdminCmdSetRotation         AdminCmd = "setrotation"
	AdminCmdSetWorkHours        AdminCmd = "setworkhours"
	AdminCmdAddTopic            AdminCmd = "addtopic"
	AdminCmdSetLabel            AdminCmd = "setlabel"
	AdminCmdCloneCategory       AdminCmd = "clonecategory"
	AdminCmdOffboard            AdminCmd = "offboard"
	AdminCmdDNS                 AdminCmd = "dns"
	AdminCmdAddPersonToCategory AdminCmd = "addpersontocategory"
)

// Step constants for the multi-step issue creation flow
const (
	StepLinearAccount = "linear_account" // ask user to link their Linear username
	StepCategory      = "category"
	StepRequestType   = "request_type"
	StepPriority      = "priority"
	StepTitle         = "title"
	StepDescription   = "description"
)

// Admin flow steps
const (
	// addcategory steps
	StepAdminCatName    = "admin_cat_name"
	StepAdminCatEmoji   = "admin_cat_emoji"
	StepAdminCatTeamKey = "admin_cat_teamkey"

	// addtype steps
	StepAdminTypeSelect = "admin_type_select"
	StepAdminTypeName   = "admin_type_name"

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

	// addcategory selection steps
	StepAdminCatSelectGroup = "admin_cat_select_group"
	StepAdminCatSelectTopic = "admin_cat_select_topic"

	// setlabel steps (DM flow: forward a user message → type label → pick group)
	StepAdminSetLabelWaitLabel = "admin_setlabel_wait_label"
	StepAdminSetLabelGroup     = "admin_setlabel_group"

	// clonecategory steps
	StepAdminCloneKeyInput = "admin_clone_key_input"

	// offboard steps
	StepOffboardUsername = "offboard_username"
	StepOffboardConfirm  = "offboard_confirm"

	// dns steps
	StepDNSMenu       = "dns_menu"
	StepDNSSelectAcct = "dns_select_acct"
	StepDNSDomain     = "dns_domain"
	StepDNSSelectRec  = "dns_select_rec"
	StepDNSRecName    = "dns_rec_name"
	StepDNSRecType    = "dns_rec_type"
	StepDNSRecValue   = "dns_rec_value"
	StepDNSRecTTL     = "dns_rec_ttl"
	StepDNSConfirm    = "dns_confirm"
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
	Priority     int // Linear priority: 1=Urgent(P0), 2=High(P1), 3=Medium(P2), 4=Low(P3)
	Title        string
	MediaLinks   []string
	MessageID    int
	ChatID       int64
	ThreadID     int

	// support flow (standalone /ticket) fields
	Description  string
	SupportMsgID int
	ChatTitle    string

	// ticket-specific fields (also reused by FlowThread for the replied-to message)
	TicketMsgLink    string
	TicketMsgBody    string
	TicketMsgDate    time.Time
	SourceMsgID      int // original replied-to message ID; used by FlowThread to forward it
	ReporterUsername string
	ReporterName     string
	ReporterUserID   int64  // Telegram user ID of the reporter; used by FlowThread to add to tech group
	RequesterLinear  string // Linear username of the person who ran /ticket; empty if not linked
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

	// setlabel fields
	LabelUserID   int64
	LabelUsername string
	LabelText     string

	// offboard fields
	OffboardUserID   int64
	OffboardUsername string
	OffboardGroupIDs []int64 // remaining groups to remove from

	// DNS management fields (experimental)
	DNSAction      string // "accounts", "list", "add", "del"
	DNSAccountID   int
	DNSDomain      string
	DNSRecordName  string
	DNSRecordType  string
	DNSRecordValue string
	DNSRecordTTL   int
	DNSRecordID    string      // selected record ID for delete flow
	DNSRecords     []dnsRecord // fetched records for delete flow
}
