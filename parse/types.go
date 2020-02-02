package parse

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/danjacques/hangouts-migrate/util"
)

type EventType string

const (
	EventTypeRenameConversation EventType = "RENAME_CONVERSATION"
	EventTypeAddUser                      = "ADD_USER"
	EventTypeRegularChatMessage           = "REGULAR_CHAT_MESSAGE"
)

type EmbedItemType string

const (
	EmbedItemPlusPhoto EmbedItemType = "PLUS_PHOTO"
	EmbedItemPlaceV2                 = "PLACE_V2"
	EmbedItemThingV2                 = "THING_V2"
	EmbedItemThing                   = "THING"
)

type ParticipantRegistry struct {
	allParticipants      []*ParticipantData
	participantsByGaiaID map[string]*ParticipantData
	participantsByChatID map[string]*ParticipantData
}

func (reg *ParticipantRegistry) Register(data *ParticipantData) {
	if id := data.ID.GaiaID; id != "" {
		if reg.participantsByGaiaID == nil {
			reg.participantsByGaiaID = make(map[string]*ParticipantData)
		}
		reg.participantsByGaiaID[id] = data
	}
	if id := data.ID.ChatID; id != "" {
		if reg.participantsByChatID == nil {
			reg.participantsByChatID = make(map[string]*ParticipantData)
		}
		reg.participantsByChatID[id] = data
	}
	reg.allParticipants = append(reg.allParticipants, data)
}

func (reg *ParticipantRegistry) AllParticipants() []*ParticipantData {
	return reg.allParticipants
}

func (reg *ParticipantRegistry) ForID(pid *ParticipantID) *ParticipantData {
	if pid.GaiaID != "" {
		if v := reg.participantsByGaiaID[pid.GaiaID]; v != nil {
			return v
		}
	}
	if pid.ChatID != "" {
		if v := reg.participantsByChatID[pid.ChatID]; v != nil {
			return v
		}
	}
	return nil
}

type Conversation struct {
	Conversation  *ConversationEntry `json:"conversation"`
	EventsMessage json.RawMessage    `json:"events"`

	initialized   bool
	reg           ParticipantRegistry
	events        []json.RawMessage
	decodedEvents []*Event
}

func (ce *Conversation) initialize() error {
	if ce.initialized {
		return nil
	}

	if err := json.Unmarshal([]byte(ce.EventsMessage), &ce.events); err != nil {
		return err
	}
	ce.decodedEvents = make([]*Event, len(ce.events))

	for _, pd := range ce.Conversation.ConversationInfo.ParticipantData {
		ce.reg.Register(pd)
	}

	ce.initialized = true
	return nil
}

func (ce *Conversation) ParticipantRegistry() *ParticipantRegistry { return &ce.reg }

func (ce *Conversation) EventsSize() int {
	return len(ce.events)
}

func (ce *Conversation) ResolveAll() error {
	for i := 0; i < len(ce.events); i++ {
		_, err := ce.Event(i)
		if err != nil {
			return err
		}
	}
	return nil
}

func (ce *Conversation) Event(i int) (*Event, error) {
	if i < 0 || i >= len(ce.events) {
		return nil, errors.New("Index out of bounds")
	}
	if ce.decodedEvents[i] == nil {
		var event Event
		if err := json.Unmarshal(ce.events[i], &event); err != nil {
			return nil, err
		}
		ce.decodedEvents[i] = &event
	}
	return ce.decodedEvents[i], nil
}

type ConversationEntry struct {
	ConversationInfo *ConversationInfo `json:"conversation"`
}

type SingleID struct {
	ID string `json:"id"`
}

type ConversationInfo struct {
	ID                 *SingleID          `json:"id"`
	Type               string             `json:"type"`
	Name               string             `json:"name"`
	CurrentParticipant []*ParticipantID   `json:"current_participant"`
	ParticipantData    []*ParticipantData `json:"participant_data"`
}

type ParticipantID struct {
	GaiaID string `json:"gaia_id"`
	ChatID string `json:"chat_id"`
}

func (pid *ParticipantID) String() string {
	return fmt.Sprintf("gaia:%s/chat:%s", pid.GaiaID, pid.ChatID)
}

// Matches returns true if pid's Gaia or Chat ID are both populated and match
// the equivalent values in other.
func (pid *ParticipantID) Matches(other *ParticipantID) bool {
	if pid.GaiaID != "" && pid.GaiaID == other.GaiaID {
		return true
	}
	if pid.ChatID != "" && pid.ChatID == other.ChatID {
		return true
	}
	return false
}

type ParticipantData struct {
	ID              ParticipantID `json:"id"`
	FallbackName    string        `json:"fallback_name"`
	ParticipantType string        `json:"participant_type"`
	DomainID        string        `json:"domain_id"`
}

func (pd *ParticipantData) DisplayName() string {
	return pd.FallbackName
}

type MessageContentSegment struct {
	Type       string `json:"type"`
	Text       string `json:"text"`
	Formatting struct {
		Bold          bool `json:"bold"`
		Italics       bool `json:"italics"`
		Strikethrough bool `json:"strikethrough"`
		Underline     bool `json:"underline"`
	} `json:"formatting"`

	LinkData *struct {
		LinkTarget string `json:"link_target"`
	} `json:"link_data"`
}

type Thumbnail struct {
	URL      string `json:"url"`
	ImageURL string `json:"image_url"`
	WidthPx  int64  `json:"width_px"`
	HeightPx int64  `json:"height_px"`
}

type PlusPhoto struct {
	Thumbnail          *Thumbnail `json:"thumbnail"`
	OwnerObfuscatedID  string     `json:"owner_obfuscated_id"`
	AlbumID            string     `json:"album_id"`
	PhotoID            string     `json:"photo_id"`
	URL                string     `json:"url"`
	OriginalContentURL string     `json:"original_content_url"`
	MediaType          string     `json:"media_type"`
}

type GeoCoordinatesV2 struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type PostalAddressV2 struct {
	StreetAddress string `json:"street_address"`

	Name            string `json:"name"`
	AddressCountry  string `json:"address_country"`
	AddressLocality string `json:"address_locality"`
	AddressRegion   string `json:"address_region"`
	PostalCode      string `json:"postal_code"`
}

type ImageObjectV2 struct {
	URL string `json:"url"`
}

type PlaceV2 struct {
	URL     string `json:"url"`
	Name    string `json:"name"`
	Address *struct {
		PostalAddressV2 PostalAddressV2 `json:"postal_address_v2"`
	} `json:"address"`
	Geo *struct {
		GeoCoordinatesV2 GeoCoordinatesV2 `json:"geo_coordinates_v2"`
	} `json:"geo"`

	ID            string         `json:"id"`
	ImageObjectV2 *ImageObjectV2 `json:"image_object_v2"`
}

type RepresentativeImage struct {
	Type          []EmbedItemType `json:"type"`
	ID            string          `json:"id"`
	ImageObjectV2 *ImageObjectV2  `json:"image_object_v2"`
}

type ThingV2 struct {
	URL                 string               `json:"url"`
	Name                string               `json:"name"`
	RepresentativeImage *RepresentativeImage `json:"representative_image"`
}

type EmbedItem struct {
	Type          []EmbedItemType `json:"type"`
	ID            string          `json:"id"`
	PlusPhoto     *PlusPhoto      `json:"plus_photo"`
	PlaceV2       *PlaceV2        `json:"place_v2"`
	ThingV2       *ThingV2        `json:"thing_v2"`
	ImageObjectV2 *ImageObjectV2  `json:"image_object_v2"`
}

func (ei *EmbedItem) Key() string {
	if pp := ei.PlusPhoto; pp != nil {
		return fmt.Sprintf("%s:%s", pp.AlbumID, pp.PhotoID)
	}
	if p := ei.ThingV2; p != nil {
		// Use a hash of the Thing's URL.
		return util.HashForKey(p.URL)
	}
	return ei.ID
}

type MessageContentAttachment struct {
	EmbedItem *EmbedItem `json:"embed_item"`
	ID        string     `json:"id"`
}

type MessageContent struct {
	Segment    []*MessageContentSegment    `json:"segment"`
	Attachment []*MessageContentAttachment `json:"attachment"`
}

type ChatMessage struct {
	MessageContent *MessageContent `json:"message_content"`
}

type ConversationRename struct {
	NewName string `json:"new_name"`
	OldName string `json:"old_name"`
}

type MembershipChange struct {
	Type          string           `json:"type"`
	ParticipantID []*ParticipantID `json:"participant_id"`
	LeaveReason   string           `json:"leave_reason"`
}

type Event struct {
	ConversationID *SingleID      `json:"conversation_id"`
	SenderID       *ParticipantID `json:"sender_id"`
	Timestamp      string         `json:"timestamp"`

	ConversationRename *ConversationRename `json:"conversation_rename"`
	ChatMessage        *ChatMessage        `json:"chat_message"`
	MembershipChange   *MembershipChange   `json:"membership_change"`

	EventID   string    `json:"event_id"`
	EventType EventType `json:"event_type"`
}

func (e *Event) Time() (time.Time, error) {
	// Timestamp is in microseconds from epoch.
	micros, err := strconv.ParseInt(e.Timestamp, 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(0, micros*1000), nil
}

func (e *Event) Description(reg *ParticipantRegistry) (string, error) {
	var parts []string

	// Time
	switch t, err := e.Time(); err {
	case nil:
		parts = append(parts, t.In(time.Local).Format(time.RFC3339Nano))
	default:
		parts = append(parts, fmt.Sprintf("Timestmap Error (%s)", e.Timestamp))
	}

	if sid := e.SenderID; sid != nil {
		var pd *ParticipantData
		if reg != nil {
			pd = reg.ForID(sid)
		}
		if pd != nil {
			parts = append(parts, fmt.Sprintf("Sender: %s", pd.DisplayName()))
		} else {
			parts = append(parts, fmt.Sprintf("Sender (UNKNOWN): %s", sid))
		}
	}

	if r := e.ConversationRename; r != nil {
		parts = append(parts, fmt.Sprintf("Rename from %q to %q", r.OldName, r.NewName))
	}
	if r := e.ChatMessage; r != nil {
		if mc := r.MessageContent; mc != nil {
			for _, s := range mc.Segment {
				parts = append(parts, s.Text)
			}
		}
	}

	return strings.Join(parts, "\n"), nil
}

func (e *Event) AllWords() []string {
	var words []string
	if r := e.ChatMessage; r != nil {
		if mc := r.MessageContent; mc != nil {
			for _, s := range mc.Segment {
				words = append(words, strings.Fields(s.Text)...)
			}
		}
	}
	return words
}

type Root struct {
	Conversations []*Conversation `json:"conversations"`

	conversationIDMap   map[string]*Conversation
	conversationNameMap map[string]string
}

func (r *Root) Decode(reader io.Reader) error {
	dec := json.NewDecoder(reader)
	if err := dec.Decode(r); err != nil {
		return err
	}

	r.conversationIDMap = make(map[string]*Conversation, len(r.Conversations))
	r.conversationNameMap = make(map[string]string, len(r.Conversations))
	for _, ce := range r.Conversations {
		if c := ce.Conversation; c != nil {
			if info := c.ConversationInfo; info != nil {
				r.conversationIDMap[info.ID.ID] = ce
				r.conversationNameMap[info.Name] = info.ID.ID
			}
		}
	}
	return nil
}

func (r *Root) GetConversationMap() map[string]string { return r.conversationNameMap }

func (r *Root) GetConversation(id string) (*Conversation, error) {
	c := r.conversationIDMap[id]
	if c == nil {
		return nil, errors.New("unknown conversation ID")
	}
	if err := c.initialize(); err != nil {
		return nil, err
	}
	return c, nil
}
