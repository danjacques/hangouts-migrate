package mattermost

import (
	"log"
	"regexp"
	"time"
)

type Emoji string

const (
	EmojiPlusOne Emoji = "+1"
	EmojiHeart         = "heart"
)

type ReactionInjector struct {
	entries []reactionInjectorEntry
}

type reactionInjectorEntry struct {
	re       *regexp.Regexp
	username string
	emoji    Emoji
}

func (ri *ReactionInjector) Add(username string, emoji Emoji, whenRegexp string) {
	re, err := regexp.Compile(whenRegexp)
	if err != nil {
		log.Printf("Could not compile reaction regexp %q: %s", whenRegexp, err)
		return
	}

	ri.entries = append(ri.entries, reactionInjectorEntry{
		re:       re,
		username: username,
		emoji:    emoji,
	})
}

func (ri *ReactionInjector) Get(text string, createdAt time.Time) []*Reaction {
	var reactions []*Reaction
	for _, e := range ri.entries {
		if e.re.MatchString(text) {
			reactions = append(reactions, &Reaction{
				User:      e.username,
				EmojiName: e.emoji,
				CreateAt:  timeToMillisFromEpoch(createdAt),
			})
		}
	}
	return reactions
}
