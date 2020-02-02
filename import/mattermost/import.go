package mattermost

// Current bulk import Version.
const currentVersion = 1

var containerOrder = []string{
	"version",
	"scheme",
	"team",
	"channel",
	"user",
	"post",
	"direct_channel",
	"direct_post",
}

var containerOrderMap map[string]int = func() map[string]int {
	m := make(map[string]int, len(containerOrder))
	for i, v := range containerOrder {
		m[v] = i
	}
	return m
}()

type BulkImportEntry interface {
	addToTypedContainer(tc *typedContainer)
}

type typedContainer struct {
	Type string `json:"type"`

	Version *int64   `json:"version,omitempty"`
	Team    *Team    `json:"team,omitempty"`
	Channel *Channel `json:"channel,omitempty"`
	User    *User    `json:"user,omitempty"`
	Post    *Post    `json:"post,omitempty"`
}

type Bool string

const (
	BoolTrue  Bool = "True"
	BoolFalse      = "False"
)

type Version struct {
	Version int64
}

func CurrentVersion() *Version {
	return &Version{Version: currentVersion}
}

func (v *Version) addToTypedContainer(tc *typedContainer) {
	tc.Type = "version"
	tc.Version = &v.Version
}

type TeamType string

const (
	TeamTypeOpen       TeamType = "O"
	TeamTypeInviteOnly          = "I"
)

type Team struct {
	Name        string   `json:"name"`
	DisplayName string   `json:"display_name"`
	Type        TeamType `json:"type"`

	Description     string `json:"description,omitempty"`
	AllowOpenInvite *bool  `json:"allow_open_invite,omitempty"`
	Scheme          string `json:"scheme,omitempty"`
}

func (t *Team) addToTypedContainer(tc *typedContainer) {
	tc.Type = "team"
	tc.Team = t
}

type ChannelType string

const (
	ChannelTypePublic  ChannelType = "O"
	ChannelTypePrivate             = "P"
)

type Channel struct {
	Team        string      `json:"team"`
	Name        string      `json:"name"`
	DisplayName string      `json:"display_name"`
	Type        ChannelType `json:"type"`

	Header  string `json:"header,omitempty"`
	Purpose string `json:"purpose,omitempty"`
	Scheme  string `json:"scheme,omitempty"`
}

func (c *Channel) addToTypedContainer(tc *typedContainer) {
	tc.Type = "channel"
	tc.Channel = c
}

type UserEmailInterval string

const (
	UserEmailIntervalImmediate UserEmailInterval = "immediate"
	UserEmailIntervalFifteen                     = "fifteen"
	UserEmailIntervalHour                        = "hour"
)

type UserRoles string

const (
	UserRoleUser  UserRoles = "system_user"
	UserRoleAdmin           = "system_admin system_user"
)

type User struct {
	Username string                `json:"username"`
	Email    string                `json:"email"`
	Role     UserRoles             `json:"role,omitempty"`
	Teams    []*UserTeamMembership `json:"teams,omitempty"`
}

func (u *User) addToTypedContainer(tc *typedContainer) {
	tc.Type = "user"
	tc.User = u
}

type TeamRoles string

const (
	TeamRoleUser  TeamRoles = "team_user"
	TeamRoleAdmin           = "team_admin team_user"
)

type UserTeamMembership struct {
	Name     string                   `json:"name"`
	Roles    TeamRoles                `json:"roles,omitempty"`
	Channels []*UserChannelMembership `json:"channels,omitempty"`
}

type ChannelRoles string

const (
	ChannelRoleUser  ChannelRoles = "channel_user"
	ChannelRoleAdmin              = "channel_admin channel_user"
)

type UserChannelMembership struct {
	Name     string       `json:"name"`
	Roles    ChannelRoles `json:"roles,omitempty"`
	Favorite *bool        `json:"favorite,omitempty"`
}

type Post struct {
	Team    string `json:"team"`
	Channel string `json:"channel"`
	User    string `json:"user"`
	Message string `json:"message"`

	// Post timestamp, in milliseconds from epoch.
	CreateAt    int64         `json:"create_at"`
	FlaggedBy   []string      `json:"flagged_by,omitempty"`
	Replies     []*Reply      `json:"replies,omitempty"`
	Reactions   []*Reaction   `json:"reaction,omitempty"`
	Attachments []*Attachment `json:"attachments,omitempty"`
}

func (p *Post) addToTypedContainer(tc *typedContainer) {
	tc.Type = "post"
	tc.Post = p
}

type Reply struct {
	User    string `json:"user"`
	Message string `json:"message"`
	// Post timestamp, in milliseconds from epoch.
	CreateAt    int64         `json:"create_at"`
	FlaggedBy   []string      `json:"flagged_by,omitempty"`
	Reactions   []*Reaction   `json:"reactions,omitempty"`
	Attachments []*Attachment `json:"attachments,omitempty"`
}

type Reaction struct {
	User      string `json:"user"`
	EmojiName Emoji  `json:"emoji_name"`
	CreateAt  int64  `json:"create_at"`
}

const MaxAttachmentsPerPost = 5

type Attachment struct {
	Path string `json:"path"`
}
