package mattermost

import (
	"fmt"
	"log"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/danjacques/hangouts-migrate/attachment"
	"github.com/danjacques/hangouts-migrate/parse"
)

type BulkImportGenerator struct {
	TeamName           string
	TeamDisplayName    string
	ChannelName        string
	ChannelDisplayName string

	UserMapper                   UserMapper
	AttachmentMapper             *attachment.Mapper
	DestAttachmentDir            string
	ReactionInjector             *ReactionInjector
	RequireAllParticipantsMapped bool
}

// BuildBulkImport creates a BulkImport object from a parsed root, r.
func (big *BulkImportGenerator) Build(c *parse.Conversation, w *BulkImportWriter) error {
	// Get a list of all users in this conversation/map.
	allUsers, err := big.allUsers(c)
	if err != nil {
		return err
	}

	// Add a version entry.
	if err := w.Add(CurrentVersion()); err != nil {
		return err
	}

	// Add a team entry for each participant.
	if err := big.AddTeamEntries(w); err != nil {
		return err
	}

	// Add a team entry for each participant.
	if err := big.AddChannelEntries(w); err != nil {
		return err
	}

	// Add a User entry for each participant.
	if err := big.AddUserEntries(w, allUsers); err != nil {
		return err
	}

	// Add a single post, with everything else a reply.
	if err := big.AddChatPost(w, c); err != nil {
		return err
	}

	return nil
}

func (big *BulkImportGenerator) allUsers(c *parse.Conversation) ([]*UserID, error) {
	var users []*UserID
	addedUsernames := make(map[string]struct{})

	// Add any users in our user map.
	for _, u := range big.UserMapper.AllUsers() {
		if _, ok := addedUsernames[u.Username]; ok {
			continue
		}
		users = append(users, u)
	}

	for _, pd := range c.ParticipantRegistry().AllParticipants() {
		// Get the associated mapped user.
		u := big.UserMapper.UserForParticipantID(&pd.ID)
		if u == nil {
			if big.RequireAllParticipantsMapped {
				return nil, fmt.Errorf("Missing required user map entry for: %s", pd.ID)
			}
			log.Printf("Skipping missing user map for: %s", pd.ID)
			continue
		}

		users = append(users, &UserID{
			Username: u.Username,
			Email:    u.Email,
			Admin:    false,
		})
		addedUsernames[u.Username] = struct{}{}
	}

	return users, nil
}

func (big *BulkImportGenerator) AddTeamEntries(w *BulkImportWriter) error {
	displayName := big.TeamDisplayName
	if displayName == "" {
		displayName = big.TeamName
	}
	w.Add(&Team{
		Name:        big.TeamName,
		DisplayName: displayName,
		Type:        TeamTypeInviteOnly,
	})
	return nil
}

func (big *BulkImportGenerator) AddChannelEntries(w *BulkImportWriter) error {
	displayName := big.ChannelDisplayName
	if displayName == "" {
		displayName = big.ChannelName
	}
	w.Add(&Channel{
		Team:        big.TeamName,
		Name:        big.ChannelName,
		DisplayName: displayName,
		Type:        ChannelTypePrivate,
	})
	return nil
}

func (big *BulkImportGenerator) AddUserEntries(w *BulkImportWriter, users []*UserID) error {
	trueBool := true
	for _, user := range users {
		userRole, teamRole, channelRole := UserRoleUser, TeamRoleUser, ChannelRoleUser
		if user.Admin {
			userRole, teamRole, channelRole = UserRoleAdmin, TeamRoleAdmin, ChannelRoleAdmin
		}

		// Augment "user" with additional membership properties.
		w.Add(&User{
			Username: user.Username,
			Email:    user.Email,
			Role:     userRole,
			Teams: []*UserTeamMembership{
				&UserTeamMembership{
					Name:  big.TeamName,
					Roles: teamRole,
					Channels: []*UserChannelMembership{
						&UserChannelMembership{
							Name:     big.ChannelName,
							Roles:    channelRole,
							Favorite: &trueBool,
						},
					},
				},
			},
		})
	}
	return nil
}

func (big *BulkImportGenerator) AddChatPost(w *BulkImportWriter, c *parse.Conversation) error {
	// Collect all events and sort by timestamp.
	type eventAndTime struct {
		Event     *parse.Event
		Timestamp time.Time
	}
	events := make([]eventAndTime, 0, c.EventsSize())
	for i := 0; i < c.EventsSize(); i++ {
		e, err := c.Event(i)
		if err != nil {
			return fmt.Errorf("Could not open event #%d: %w", i, err)
		}

		// Only care about chat messages.
		if e.EventType != parse.EventTypeRegularChatMessage {
			continue
		}

		ts, err := e.Time()
		if err != nil {
			return fmt.Errorf("Could not get timestamp for event #%d: %w", i, err)
		}
		events = append(events, eventAndTime{e, ts})
	}
	sort.Slice(events, func(i, j int) bool {
		return events[i].Timestamp.Before(events[j].Timestamp)
	})

	// Use the first event as the initial Post.
	var lastTextPost *Post
	for _, e := range events {
		u := big.UserMapper.UserForParticipantID(e.Event.SenderID)
		if u == nil {
			desc, err := e.Event.Description(c.ParticipantRegistry())
			if err != nil {
				desc = fmt.Sprintf("ERROR(%s)", err)
			}
			log.Printf("ERROR: Skipping post by unmapped sender %s:\n%s", e.Event.SenderID, desc)
			continue
		}

		text := messageForEvent(e.Event)
		attachments := big.attachmentsForEvent(e.Event)

		// If this is an attachment-only event, and it shares a username with the
		// last text post, then try and augment the last text post with
		// these attachments.
		if lastTextPost != nil && text == "" && u.Username == lastTextPost.User {
			for len(attachments) > 0 && len(lastTextPost.Attachments) <= MaxAttachmentsPerPost {
				lastTextPost.Attachments = append(lastTextPost.Attachments, attachments[0])
				attachments = attachments[1:]
			}
		}

		if text == "" && len(attachments) == 0 {
			// Empty event.
			continue
		}

		p := &Post{
			Team:        big.TeamName,
			Channel:     big.ChannelName,
			User:        u.Username,
			Message:     text,
			CreateAt:    timeToMillisFromEpoch(e.Timestamp),
			Attachments: attachments,
		}
		if text != "" && big.ReactionInjector != nil {
			// Reaction timestaho has to exceed the post timestamp. Add a minute.
			p.Reactions = big.ReactionInjector.Get(text, e.Timestamp.Add(time.Minute))
		}
		w.Add(p)

		if text != "" {
			lastTextPost = p
		}
	}
	return nil
}

func (big *BulkImportGenerator) attachmentsForEvent(e *parse.Event) []*Attachment {
	if e.ChatMessage == nil || e.ChatMessage.MessageContent == nil {
		return nil
	}

	var attachments []*Attachment
	for _, a := range e.ChatMessage.MessageContent.Attachment {
		if a.EmbedItem.PlusPhoto == nil {
			continue
		}

		path := big.AttachmentMapper.GetPath(a.EmbedItem.Key())
		if path == "" {
			log.Printf("ERROR: Skipping unmapped attachment %q", a.EmbedItem.Key())
			continue
		}

		// Convert from source path to destination path.
		path = filepath.Join(big.DestAttachmentDir, filepath.Base(path))

		attachments = append(attachments, &Attachment{
			Path: path,
		})
	}
	return attachments
}

func messageForEvent(e *parse.Event) string {
	if e.ChatMessage == nil || e.ChatMessage.MessageContent == nil {
		return ""
	}

	var lines []string
	for _, seg := range e.ChatMessage.MessageContent.Segment {
		lines = append(lines, seg.Text)
	}
	return strings.Join(lines, "\n")
}

func timeToMillisFromEpoch(t time.Time) int64 {
	const millisecondsInANanosecond = int64(time.Millisecond / time.Nanosecond)
	return t.UnixNano() / millisecondsInANanosecond
}
