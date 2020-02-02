package mattermost

import (
	"encoding/json"
	"io"

	"github.com/danjacques/hangouts-migrate/parse"
)

type UserID struct {
	Username string
	Email    string
	Admin    bool
}

type UserMapper interface {
	UserForParticipantID(pid *parse.ParticipantID) *UserID
	AllUsers() []*UserID
}

type FixedUserMapper struct {
	entriesByChatID map[string]*UserID
	entriesByGaiaID map[string]*UserID
	allUsers        []*UserID
}

var _ UserMapper = (*FixedUserMapper)(nil)

func (fum *FixedUserMapper) Register(chatID, gaiaID string, user *UserID) {
	if chatID != "" {
		if fum.entriesByChatID == nil {
			fum.entriesByChatID = make(map[string]*UserID)
		}
		fum.entriesByChatID[chatID] = user
	}

	if gaiaID != "" {
		if fum.entriesByGaiaID == nil {
			fum.entriesByGaiaID = make(map[string]*UserID)
		}
		fum.entriesByGaiaID[gaiaID] = user
	}
	fum.allUsers = append(fum.allUsers, user)
}

func (fum *FixedUserMapper) AllUsers() []*UserID { return fum.allUsers }

func (fum *FixedUserMapper) UserForParticipantID(pid *parse.ParticipantID) *UserID {
	if u := fum.entriesByChatID[pid.ChatID]; u != nil {
		return u
	}
	return fum.entriesByGaiaID[pid.GaiaID]
}

type fixedUserMapperEntry struct {
	ChatID string `json:"chat_id"`
	GaiaID string `json:"gaia_id"`

	Username string `json:"username"`
	Email    string `json:"email"`

	Admin bool `json:"admin"`

	Reactions []*autoReaction `json:"auto_reactions,omitempty"`
}

type autoReaction struct {
	Regexp string `json:"regexp"`
	Emoji  string `json:"emoji"`
}

func LoadFixedUserMapFromJSON(r io.Reader, ri *ReactionInjector) (*FixedUserMapper, error) {
	var entries []*fixedUserMapperEntry
	dec := json.NewDecoder(r)
	if err := dec.Decode(&entries); err != nil {
		return nil, err
	}

	var fum FixedUserMapper
	for _, e := range entries {
		fum.Register(e.ChatID, e.GaiaID, &UserID{
			Username: e.Username,
			Email:    e.Email,
			Admin:    e.Admin,
		})

		if ri != nil {
			for _, ar := range e.Reactions {
				ri.Add(e.Username, Emoji(ar.Emoji), ar.Regexp)
			}
		}
	}
	return &fum, nil
}

func SerializeParticipantsToJSON(pr *parse.ParticipantRegistry, w io.Writer) error {
	entries := make([]fixedUserMapperEntry, 0, len(pr.AllParticipants()))
	for _, p := range pr.AllParticipants() {
		entries = append(entries, fixedUserMapperEntry{
			ChatID:   p.ID.ChatID,
			GaiaID:   p.ID.GaiaID,
			Username: p.DisplayName(),
		})
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(entries)
}
