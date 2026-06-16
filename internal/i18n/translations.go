package i18n

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"
)

// Lang represents a language code
type Lang string

const (
	English Lang = "eng"
	Russian Lang = "ru"
)

// Translations holds all messages for a language
type Translations struct {
	Thread       ThreadMessages
	Ticket       TicketMessages
	Category     CategoryMessages
	Person       PersonMessages
	Admin        AdminMessages
	DNS          DNSMessages
	Error        ErrorMessages
	Common       CommonMessages
	OnDuty       OnDutyMessages
	Rotation     RotationMessages
	RequestType  RequestTypeMessages
	Group        GroupMessages
	User         UserMessages
}

type ThreadMessages struct {
	SelectCategory                string `yaml:"selectCategory"`
	CreatedSuccess               string `yaml:"createdSuccess"`
	CreatedSuccessNext           string `yaml:"createdSuccessNext"`
	JoinGroup                    string `yaml:"joinGroup"`
	GoToTopic                    string `yaml:"goToTopic"`
	LinearIssue                  string `yaml:"linearIssue"`
	OpenLinearIssue              string `yaml:"openLinearIssue"`
	ThreadClosed                 string `yaml:"threadClosed"`
	UploadComplete               string `yaml:"uploadComplete"`
	UploadCompleteWithFiles      string `yaml:"uploadCompleteWithFiles"`
	ReplyRequired                string `yaml:"replyRequired"`
	ReplyToUserMessage           string `yaml:"replyToUserMessage"`
	NoCategories                 string `yaml:"noCategories"`
	NotInTechTopic               string `yaml:"notInTechTopic"`
	TechGroupNotConfigured       string `yaml:"techGroupNotConfigured"`
	MentionNotification          string `yaml:"mentionNotification"`
	OnCall                       string `yaml:"onCall"`
	OnCallUnassigned             string `yaml:"onCallUnassigned"`
	PingName                     string `yaml:"pingName"`
	AssignedPersonOfflineWarning string `yaml:"assignedPersonOfflineWarning"`
	UseCloseToDump               string `yaml:"useCloseToDump"`
	MediaTooLarge                string `yaml:"mediaTooLarge"`
	TelegramThread               string `yaml:"telegramThread"`
	ClosedByOn                   string `yaml:"closedByOn"`
}

type TicketMessages struct {
	SelectCategory            string `yaml:"selectCategory"`
	CreatedSuccess            string `yaml:"createdSuccess"`
	ReplyRequired             string `yaml:"replyRequired"`
	ReplyToUserMessage        string `yaml:"replyToUserMessage"`
	MustBeInGroup             string `yaml:"mustBeInGroup"`
	DescribeIssue             string `yaml:"describeIssue"`
	DescribeFromScratch       string `yaml:"describeFromScratch"`
	DescriptionSaved          string `yaml:"descriptionSaved"`
	SelectCategory2           string `yaml:"selectCategory2"`
	SelectRequestType         string `yaml:"selectRequestType"`
	SelectPriority            string `yaml:"selectPriority"`
	NoCategories              string `yaml:"noCategories"`
	UnderCategory             string `yaml:"underCategory"`
}

type CategoryMessages struct {
	EnterName                  string `yaml:"enterName"`
	EnterEmoji                 string `yaml:"enterEmoji"`
	EnterLinearTeamKey         string `yaml:"enterLinearTeamKey"`
	AddedSuccess               string `yaml:"addedSuccess"`
	SelectGroup                string `yaml:"selectGroup"`
	SelectTopic                string `yaml:"selectTopic"`
	SelectType                 string `yaml:"selectType"`
	SelectRotationType         string `yaml:"selectRotationType"`
	EnterNewRequestTypeName    string `yaml:"enterNewRequestTypeName"`
	SelectPerson               string `yaml:"selectPerson"`
	ConfirmDeleteCategory      string `yaml:"confirmDeleteCategory"`
	NoCategories               string `yaml:"noCategories"`
	FailedToAdd                string `yaml:"failedToAdd"`
	FailedToUpdate             string `yaml:"failedToUpdate"`
	FailedToLoad               string `yaml:"failedToLoad"`
	NoApprovedGroups           string `yaml:"noApprovedGroups"`
	DeletedSuccessfully        string `yaml:"deletedSuccessfully"`
	LinkedTypeSuccess          string `yaml:"linkedTypeSuccess"`
	RotationUpdated            string `yaml:"rotationUpdated"`
	SelectWhoTakingOver        string `yaml:"selectWhoTakingOver"`
	PriorityP0                 string `yaml:"priorityP0"`
	PriorityP1                 string `yaml:"priorityP1"`
	PriorityP2                 string `yaml:"priorityP2"`
	PriorityP3                 string `yaml:"priorityP3"`
	TodayOnly                  string `yaml:"todayOnly"`
	UntilFriday                string `yaml:"untilFriday"`
	ThisWeek                   string `yaml:"thisWeek"`
	CancelTakeover             string `yaml:"cancelTakeover"`
	ConfirmDelete              string `yaml:"confirmDelete"`
	CloneCategory              string `yaml:"cloneCategory"`
	CloningPrompt              string `yaml:"cloningPrompt"`
	EnterNewTeamKey            string `yaml:"enterNewTeamKey"`
	EditName                   string `yaml:"editName"`
	EditEmoji                  string `yaml:"editEmoji"`
	EditKey                    string `yaml:"editKey"`
	MakeGlobal                 string `yaml:"makeGlobal"`
	GroupLevel                 string `yaml:"groupLevel"`
	TopicLevel                 string `yaml:"topicLevel"`
}

type PersonMessages struct {
	EnterName                 string `yaml:"enterName"`
	EnterTelegramUsername     string `yaml:"enterTelegramUsername"`
	EnterLinearUsername       string `yaml:"enterLinearUsername"`
	SelectTimezone            string `yaml:"selectTimezone"`
	EnterTimezone             string `yaml:"enterTimezone"`
	EnterWorkHours            string `yaml:"enterWorkHours"`
	SelectWorkHours           string `yaml:"selectWorkHours"`
	EnterWorkDays             string `yaml:"enterWorkDays"`
	SelectWorkDays            string `yaml:"selectWorkDays"`
	AddedSuccess              string `yaml:"addedSuccess"`
	WorkHoursUpdated          string `yaml:"workHoursUpdated"`
	ConfirmDelete             string `yaml:"confirmDelete"`
	FailedToAdd               string `yaml:"failedToAdd"`
	FailedToLoad              string `yaml:"failedToLoad"`
	FailedToDelete            string `yaml:"failedToDelete"`
	PersonNotFound            string `yaml:"personNotFound"`
	NoPersonsAvailable        string `yaml:"noPersonsAvailable"`
	NoPersonsYet              string `yaml:"noPersonsYet"`
	SelectPerson              string `yaml:"selectPerson"`
	AddPerson                 string `yaml:"addPerson"`
	EditSchedule              string `yaml:"editSchedule"`
	RemovePerson              string `yaml:"removePerson"`
	DeletePerson              string `yaml:"deletePerson"`
	LinearAccountSet          string `yaml:"linearAccountSet"`
	CurrentLinearAccount      string `yaml:"currentLinearAccount"`
	EnterNewUsername          string `yaml:"enterNewUsername"`
	OffboardUsername          string `yaml:"offboardUsername"`
}

type AdminMessages struct {
	GroupNotifications        string `yaml:"groupNotifications"`
	PendingNotifications      string `yaml:"pendingNotifications"`
	ApprovedGroups            string `yaml:"approvedGroups"`
	PendingGroups             string `yaml:"pendingGroups"`
	AutoRegisteredNote        string `yaml:"autoRegisteredNote"`
	NoGroupsRegistered        string `yaml:"noGroupsRegistered"`
	SelectGroup               string `yaml:"selectGroup"`
	ApproveGroup              string `yaml:"approveGroup"`
	RejectGroup               string `yaml:"rejectGroup"`
	RegisteredTopics          string `yaml:"registeredTopics"`
	NoTopicsYet               string `yaml:"noTopicsYet"`
	TopicRegistered           string `yaml:"topicRegistered"`
	AddTopicName              string `yaml:"addTopicName"`
	AddTopicID                string `yaml:"addTopicID"`
	KnownUsers                string `yaml:"knownUsers"`
	NoUsersYet                string `yaml:"noUsersYet"`
	SetUserLabel              string `yaml:"setUserLabel"`
	TelegramUser              string `yaml:"telegramUser"`
	LinearUser                string `yaml:"linearUser"`
	LabelTooLong              string `yaml:"labelTooLong"`
	TagSet                    string `yaml:"tagSet"`
	ClearTagForUser           string `yaml:"clearTagForUser"`
	Categories                string `yaml:"categories"`
	RotationOverview          string `yaml:"rotationOverview"`
	FailedToLoad              string `yaml:"failedToLoad"`
	FailedToDelete            string `yaml:"failedToDelete"`
	Cancelled                 string `yaml:"cancelled"`
	ExportFailed              string `yaml:"exportFailed"`
	ListingGroupsFailed       string `yaml:"listingGroupsFailed"`
	NotMemberOfGroups         string `yaml:"notMemberOfGroups"`
	UserNotInDatabase         string `yaml:"userNotInDatabase"`
	Back                      string `yaml:"back"`
	Cancel                    string `yaml:"cancel"`
	Confirm                   string `yaml:"confirm"`
}

type DNSMessages struct {
	NotConfigured    string `yaml:"notConfigured"`
	SelectAccount    string `yaml:"selectAccount"`
	SelectAction     string `yaml:"selectAction"`
	ListRecords      string `yaml:"listRecords"`
	AddRecord        string `yaml:"addRecord"`
	DeleteRecord     string `yaml:"deleteRecord"`
	EnterDomain      string `yaml:"enterDomain"`
	EnterRecordName  string `yaml:"enterRecordName"`
	EnterRecordValue string `yaml:"enterRecordValue"`
	EnterTTL         string `yaml:"enterTTL"`
	InvalidTTL       string `yaml:"invalidTTL"`
	CreateRecord     string `yaml:"createRecord"`
	DeleteRecord2    string `yaml:"deleteRecord2"`
	RecordCreated    string `yaml:"recordCreated"`
	RecordDeleted    string `yaml:"recordDeleted"`
	NoRecordsFound   string `yaml:"noRecordsFound"`
	FailedFetchAccts string `yaml:"failedFetchAccts"`
	FailedSelectAcct string `yaml:"failedSelectAcct"`
	FailedFetchRecs  string `yaml:"failedFetchRecs"`
	FailedCreateRec  string `yaml:"failedCreateRec"`
	FailedDeleteRec  string `yaml:"failedDeleteRec"`
}

type ErrorMessages struct {
	Failed                      string `yaml:"failed"`
	InvalidTimezone             string `yaml:"invalidTimezone"`
	InvalidWorkDays             string `yaml:"invalidWorkDays"`
	InvalidWorkHours            string `yaml:"invalidWorkHours"`
	LinearUsernameEmpty         string `yaml:"linearUsernameEmpty"`
	FailedCreateIssue           string `yaml:"failedCreateIssue"`
	FailedCreateTopic           string `yaml:"failedCreateTopic"`
	FailedLoadCategories        string `yaml:"failedLoadCategories"`
	FailedLoadPersons           string `yaml:"failedLoadPersons"`
	FailedLoadGroups            string `yaml:"failedLoadGroups"`
	FailedAddCategory           string `yaml:"failedAddCategory"`
	FailedCreateAssignment      string `yaml:"failedCreateAssignment"`
	FailedAddRequestType        string `yaml:"failedAddRequestType"`
	FailedLinkType              string `yaml:"failedLinkType"`
	FailedSetRotation           string `yaml:"failedSetRotation"`
	FailedAddToCategory         string `yaml:"failedAddToCategory"`
	FailedRemove                string `yaml:"failedRemove"`
	FailedDelete                string `yaml:"failedDelete"`
	FailedLoadPersonsList       string `yaml:"failedLoadPersonsList"`
	ClonedFailed                string `yaml:"clonedFailed"`
	DBError                     string `yaml:"dbError"`
	NoOpenThread                string `yaml:"noOpenThread"`
	FailedSendMessage           string `yaml:"failedSendMessage"`
}

type CommonMessages struct {
	Loading                  string `yaml:"loading"`
	Selected                 string `yaml:"selected"`
	Prev                     string `yaml:"prev"`
	Next                     string `yaml:"next"`
	KeepOption               string `yaml:"keepOption"`
	Updated                 string `yaml:"updated"`
	CategoryNotFound         string `yaml:"categoryNotFound"`
}

type OnDutyMessages struct {
	OnCall                   string `yaml:"onCall"`
	OnCallUnassigned         string `yaml:"onCallUnassigned"`
	OnDutySupport            string `yaml:"onDutySupport"`
	SupportPersons           string `yaml:"supportPersons"`
	NoSupportPersonsYet      string `yaml:"noSupportPersonsYet"`
}

type RotationMessages struct {
	SelectRotationType       string `yaml:"selectRotationType"`
	SettingRotation          string `yaml:"settingRotation"`
	GroupSelected            string `yaml:"groupSelected"`
	TopicSelected            string `yaml:"topicSelected"`
	GlobalScope              string `yaml:"globalScope"`
}

type RequestTypeMessages struct {
	AddedSuccess             string `yaml:"addedSuccess"`
	SelectRequestType        string `yaml:"selectRequestType"`
}

type GroupMessages struct {
	RegisterGroups           string `yaml:"registerGroups"`
	SelectGroup              string `yaml:"selectGroup"`
	SelectTopic              string `yaml:"selectTopic"`
}

type UserMessages struct {
	LookupFailed             string `yaml:"lookupFailed"`
	NotFound                 string `yaml:"notFound"`
}

// Load loads translations for a given language
func Load(lang Lang) (*Translations, error) {
	langCode := string(lang)
	filename := filepath.Join("resources", "i18n", langCode+".yaml")

	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to load translations for %s from %s: %w", langCode, filename, err)
	}

	var t Translations
	if err := yaml.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("failed to parse translations for %s: %w", langCode, err)
	}

	return &t, nil
}

// Manager holds loaded translations for multiple languages and provides safe access
type Manager struct {
	translations map[Lang]*Translations
	mu           sync.RWMutex
	defaultLang  Lang
}

// NewManager creates a new translation manager
func NewManager(defaultLang Lang) *Manager {
	return &Manager{
		translations: make(map[Lang]*Translations),
		defaultLang:  defaultLang,
	}
}

// LoadLanguage loads a language into the manager
func (m *Manager) LoadLanguage(lang Lang) error {
	t, err := Load(lang)
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.translations[lang] = t
	m.mu.Unlock()
	return nil
}

// Get returns translations for a language, defaulting to English if not found
func (m *Manager) Get(lang Lang) *Translations {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if t, ok := m.translations[lang]; ok {
		return t
	}
	if t, ok := m.translations[English]; ok {
		return t
	}
	// Should never happen if English is loaded
	return &Translations{}
}
