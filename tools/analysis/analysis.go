package analysis

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/danjacques/hangouts-migrate/attachment"
	"github.com/danjacques/hangouts-migrate/import/mattermost"
	"github.com/danjacques/hangouts-migrate/parse"
	"github.com/google/subcommands"
)

func withBufferedReader(path string, fn func(io.Reader) error) error {
	fd, err := os.Open(path)
	if err != nil {
		return err
	}
	defer fd.Close()

	br := bufio.NewReader(fd)
	return fn(br)
}

func withBufferedWriter(path string, fn func(io.Writer) error) error {
	fd, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("could not open %q: %w", path, err)
	}
	defer func() {
		if fd != nil {
			fd.Close()
		}
	}()

	bfd := bufio.NewWriter(fd)
	if err := fn(bfd); err != nil {
		return err
	}
	if err := bfd.Flush(); err != nil {
		return fmt.Errorf("could not flush buffered writer: %w", err)
	}
	if err := fd.Close(); err != nil {
		return fmt.Errorf("could not close file: %w", err)
	}
	fd = nil // Do not close in defer.
	return nil
}

func loadRoot(path string) (*parse.Root, error) {
	var root parse.Root
	err := withBufferedReader(path, func(r io.Reader) error {
		log.Println("Loading root document...")
		if err := root.Decode(r); err != nil {
			return err
		}
		log.Println("Successfully loaded root document!")
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &root, nil
}

func loadUserMapJSON(path string) (*mattermost.FixedUserMapper, *mattermost.ReactionInjector, error) {
	var um *mattermost.FixedUserMapper
	var ri mattermost.ReactionInjector
	err := withBufferedReader(path, func(r io.Reader) (err error) {
		um, err = mattermost.LoadFixedUserMapFromJSON(r, &ri)
		return
	})
	if err != nil {
		return nil, nil, err
	}
	return um, &ri, nil
}

type listChatsCommand struct {
	path string
}

func (cmd *listChatsCommand) Name() string     { return "list-chats" }
func (cmd *listChatsCommand) Synopsis() string { return "List names and IDs of chats." }
func (cmd *listChatsCommand) Usage() string {
	return `list-chats -path /path/to/JSON.json
	List chats in <path>.
	`
}

func (cmd *listChatsCommand) SetFlags(f *flag.FlagSet) {
	f.StringVar(&cmd.path, "path", "", "Path to the Hangouts JSON file.")
}

func (cmd *listChatsCommand) Execute(_ context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	r, err := loadRoot(cmd.path)
	if err != nil {
		log.Printf("ERROR: Failed to load root file: %s", err)
		return subcommands.ExitFailure
	}

	names := make([]string, len(r.GetConversationMap()))
	for k, v := range r.GetConversationMap() {
		names = append(names, fmt.Sprintf("%s: %s", k, v))
	}
	sort.Strings(names)
	for _, name := range names {
		log.Print(name)
	}
	return subcommands.ExitSuccess
}

type dumpChatCommand struct {
	path           string
	conversationID string
}

func (cmd *dumpChatCommand) Name() string     { return "dump-chat" }
func (cmd *dumpChatCommand) Synopsis() string { return "Dumps the text output of a chat." }
func (cmd *dumpChatCommand) Usage() string {
	return `dump-chat -path /path/to/JSON.json -conversation ID
	Dump the contents of conversation <ID>.
	`
}

func (cmd *dumpChatCommand) SetFlags(f *flag.FlagSet) {
	f.StringVar(&cmd.path, "path", "", "Path to the Hangouts JSON file.")
	f.StringVar(&cmd.conversationID, "conversation", "", "Conversation ID to dump.")
}

func (cmd *dumpChatCommand) Execute(_ context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	if cmd.conversationID == "" {
		log.Printf("ERROR: you must supply a conversation ID (-conversation).")
		return subcommands.ExitFailure
	}

	r, err := loadRoot(cmd.path)
	if err != nil {
		log.Printf("ERROR: Failed to load root file: %s", err)
		return subcommands.ExitFailure
	}

	c, err := r.GetConversation(cmd.conversationID)
	if err != nil {
		log.Printf("Could not get conversation %q: %s", cmd.conversationID, err)
		return subcommands.ExitFailure
	}

	for i := 0; i < c.EventsSize(); i++ {
		e, err := c.Event(i)
		if err != nil {
			log.Printf("Event #%d could not be processed: %s", i, err)
			continue
		}

		s, err := e.Description(c.ParticipantRegistry())
		if err != nil {
			log.Printf("Event #%d could not be described: %s", i, err)
			continue
		}

		fmt.Printf("Event #%d\n%s\n\n", i, s)
	}

	log.Println("Finished!")
	return subcommands.ExitSuccess
}

type donwloadAttachmentsCommand struct {
	path           string
	out            string
	conversationID string
	attachmentPath string

	cookiePath string
	overwrite  bool
}

func (cmd *donwloadAttachmentsCommand) Name() string { return "download-attachments" }
func (cmd *donwloadAttachmentsCommand) Synopsis() string {
	return "Downloads and catalogues the attachments in a conversation."
}
func (cmd *donwloadAttachmentsCommand) Usage() string {
	return `download-attachments
	Dump the contents of conversation <ID>.
	`
}

func (cmd *donwloadAttachmentsCommand) SetFlags(f *flag.FlagSet) {
	f.StringVar(&cmd.path, "path", "", "Path to the Hangouts JSON file.")
	f.StringVar(&cmd.out, "out", "", "Path to the attachments output JSON file.")
	f.StringVar(&cmd.conversationID, "conversation", "", "Conversation ID to dump.")
	f.StringVar(&cmd.attachmentPath, "attachment_path", "", "If provided, download images here.")
	f.StringVar(&cmd.cookiePath, "cookie_path", "", "Path to the cookie JSON file to use.")
	f.BoolVar(&cmd.overwrite, "overwrite", false, "Ignore existing download state.")
}

func (cmd *donwloadAttachmentsCommand) Execute(_ context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	if cmd.conversationID == "" {
		log.Printf("ERROR: you must supply a conversation ID (-conversation).")
		return subcommands.ExitFailure
	}

	// If |out| already exists, load it.
	am := attachment.Mapper{
		BasePath:  cmd.attachmentPath,
		Overwrite: cmd.overwrite,
	}
	imageDownload := &parse.ImageDownloader{
		AttachmentMapper: &am,
		Concurrency:      5,
	}

	if cmd.cookiePath != "" {
		err := withBufferedReader(cmd.cookiePath, func(r io.Reader) (err error) {
			imageDownload.Cookies, err = parse.LoadCookieJarFromText(r)
			return
		})
		if err != nil {
			log.Printf("Could not load cookie jar from %s: %s", cmd.cookiePath, err)
			return subcommands.ExitFailure
		}
		log.Printf("Loaded %d cookie(s)", len(imageDownload.Cookies))
	}

	if !cmd.overwrite {
		if _, err := os.Stat(cmd.out); err == nil {
			err = withBufferedReader(cmd.out, func(r io.Reader) error {
				return am.LoadFromJSON(r)
			})
			if err != nil {
				log.Printf("Could not load images from %s: %s", cmd.out, err)
				return subcommands.ExitFailure
			}
		} else if !os.IsNotExist(err) {
			log.Printf("ERROR: Could not stat output path %s: %s", cmd.out, err)
			return subcommands.ExitFailure
		}
	}

	r, err := loadRoot(cmd.path)
	if err != nil {
		log.Printf("ERROR: Failed to load root file: %s", err)
		return subcommands.ExitFailure
	}

	c, err := r.GetConversation(cmd.conversationID)
	if err != nil {
		log.Printf("Could not get conversation %q: %s", cmd.conversationID, err)
		return subcommands.ExitFailure
	}

	if err := os.MkdirAll(cmd.attachmentPath, 0755); err != nil {
		log.Printf("ERROR: Could not create image path: %s", err)
		return subcommands.ExitFailure
	}

	flushAttachments := func() error {
		return withBufferedWriter(cmd.out, func(w io.Writer) error {
			return am.SaveToJSON(w)
		})
	}

	const flushInterval = 100
	added := 0
	nextFlush := flushInterval

	for i := 0; i < c.EventsSize(); i++ {
		e, err := c.Event(i)
		if err != nil {
			log.Printf("Event #%d could not be processed: %s", i, err)
			continue
		}

		if cm := e.ChatMessage; cm != nil {
			if mc := cm.MessageContent; mc != nil {
				for _, a := range mc.Attachment {
					if ei := a.EmbedItem; ei != nil {
						if imageDownload.Add(ei) {
							added++
						}
					}
				}
			}
		}

		if added > nextFlush {
			if err := flushAttachments(); err != nil {
				log.Printf("ERROR: Failed to flush attachments: %s", err)
				return subcommands.ExitFailure
			}
			nextFlush = added + flushInterval
		}
	}

	if imageDownload != nil {
		log.Println("Waiting for images to download...")
		imageDownload.Wait()
	}
	log.Println("Finished!")

	if err := flushAttachments(); err != nil {
		log.Printf("ERROR: Could not write output to %s: %s", cmd.out, err)
		return subcommands.ExitFailure
	}

	return subcommands.ExitSuccess
}

type generateUserList struct {
	path string
	out  string

	conversationID string
}

func (cmd *generateUserList) Name() string { return "generate-user-list" }
func (cmd *generateUserList) Synopsis() string {
	return "Generates a MatterMost import user list from all entries in a Hangouts.json."
}
func (cmd *generateUserList) Usage() string {
	return `generate-user-list [flags]
	Generate a MatterMost chat dump.
	`
}

func (cmd *generateUserList) SetFlags(f *flag.FlagSet) {
	f.StringVar(&cmd.path, "path", "", "Path to the Hangouts JSON file.")
	f.StringVar(&cmd.out, "out", "", "Destination output path.")
	f.StringVar(&cmd.conversationID, "conversation", "", "Conversation ID to dump.")
}

func (cmd *generateUserList) Execute(_ context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	r, err := loadRoot(cmd.path)
	if err != nil {
		log.Printf("ERROR: Failed to load root file: %s", err)
		return subcommands.ExitFailure
	}

	c, err := r.GetConversation(cmd.conversationID)
	if err != nil {
		log.Printf("Could not get conversation %q: %s", cmd.conversationID, err)
		return subcommands.ExitFailure
	}

	err = withBufferedWriter(cmd.out, func(w io.Writer) error {
		return mattermost.SerializeParticipantsToJSON(c.ParticipantRegistry(), w)
	})
	if err != nil {
		log.Printf("Could not serialize users: %s", err)
		return subcommands.ExitFailure
	}
	return subcommands.ExitSuccess
}

type generateBulkImport struct {
	path string
	out  string

	conversationID       string
	attachmentMapJSON    string
	remoteAttachmentPath string
	userMapPath          string

	mmTeamName           string
	mmTeamDisplayName    string
	mmChannelName        string
	mmChannelDisplayName string
}

func (cmd *generateBulkImport) Name() string     { return "generate-bulk-import" }
func (cmd *generateBulkImport) Synopsis() string { return "Generates a MatterMost chat dump." }
func (cmd *generateBulkImport) Usage() string {
	return `generate-bulk-import [flags]
	Generate a MatterMost chat dump.
	`
}

func (cmd *generateBulkImport) SetFlags(f *flag.FlagSet) {
	f.StringVar(&cmd.path, "path", "", "Path to the Hangouts JSON file.")
	f.StringVar(&cmd.out, "out", "", "Destination output path.")
	f.StringVar(&cmd.conversationID, "conversation", "", "Conversation ID to dump.")
	f.StringVar(&cmd.attachmentMapJSON, "attachment_map_json", "", "The JSON mapping for attachment keys to files.")
	f.StringVar(&cmd.remoteAttachmentPath, "remote_attachment_path", "", "If provided, download images here.")
	f.StringVar(&cmd.userMapPath, "user_map_path", "", "The Hangout username to MatterMost ID map JSON.")

	f.StringVar(&cmd.mmTeamName, "mm_team_name", "", "The destination MatterMots team name.")
	f.StringVar(&cmd.mmTeamDisplayName, "mm_team_display_name", "", "The destination MatterMost team display name.")
	f.StringVar(&cmd.mmChannelName, "mm_channel_name", "", "The destination MatterMost channel name.")
	f.StringVar(&cmd.mmChannelDisplayName, "mm_channel_display_name", "", "The destination MatterMost channel display name.")
}

func (cmd *generateBulkImport) Execute(_ context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	userMapper, reactionInjector, err := loadUserMapJSON(cmd.userMapPath)
	if err != nil {
		log.Printf("ERROR: Failed to load usermap from %s: %s", cmd.userMapPath, err)
		return subcommands.ExitFailure
	}

	r, err := loadRoot(cmd.path)
	if err != nil {
		log.Printf("ERROR: Failed to load root file: %s", err)
		return subcommands.ExitFailure
	}

	c, err := r.GetConversation(cmd.conversationID)
	if err != nil {
		log.Printf("Could not get conversation %q: %s", cmd.conversationID, err)
		return subcommands.ExitFailure
	}

	am := attachment.Mapper{}
	err = withBufferedReader(cmd.attachmentMapJSON, func(r io.Reader) error {
		return am.LoadFromJSON(r)
	})
	if err != nil {
		log.Printf("Could not load attachment map from %q: %s", cmd.attachmentMapJSON, err)
		return subcommands.ExitFailure
	}

	big := mattermost.BulkImportGenerator{
		TeamName:           cmd.mmTeamName,
		TeamDisplayName:    cmd.mmTeamDisplayName,
		ChannelName:        cmd.mmChannelName,
		ChannelDisplayName: cmd.mmChannelDisplayName,
		UserMapper:         userMapper,
		AttachmentMapper:   &am,
		ReactionInjector:   reactionInjector,
		DestAttachmentDir:  cmd.remoteAttachmentPath,
	}

	err = withBufferedWriter(cmd.out, func(w io.Writer) error {
		biw := mattermost.NewWriter(w)
		if err := big.Build(c, biw); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		log.Printf("Failed to serialize bulk import to JSONL: %s", err)
		return subcommands.ExitFailure
	}
	return subcommands.ExitSuccess
}

type printAllText struct {
	path string
	out  string

	conversationID     string
	chatID             string
	excludeRegexpsPath string
}

func (cmd *printAllText) Name() string     { return "print-all-text" }
func (cmd *printAllText) Synopsis() string { return "Prints all text, back to back." }
func (cmd *printAllText) Usage() string {
	return `print-all-text [flags]
	Print all text, for word cloud.
	`
}

func (cmd *printAllText) SetFlags(f *flag.FlagSet) {
	f.StringVar(&cmd.path, "path", "", "Path to the Hangouts JSON file.")
	f.StringVar(&cmd.out, "out", "", "Destination output path.")
	f.StringVar(&cmd.conversationID, "conversation", "", "Conversation ID to dump.")
	f.StringVar(&cmd.chatID, "chat_id", "", "Isolate to just this chat ID.")
	f.StringVar(&cmd.excludeRegexpsPath, "exclude_regexps", "", "Path of an exclude regexp list.")
}

var alphaNumeric = regexp.MustCompile("[^A-Za-z0-9]+")

func (cmd *printAllText) Execute(_ context.Context, f *flag.FlagSet, _ ...interface{}) subcommands.ExitStatus {
	excludeRegexp, err := cmd.buildExcludeRegexp()
	if err != nil {
		log.Printf("Could not compile exclude regexps: %s", err)
		return subcommands.ExitFailure
	}

	r, err := loadRoot(cmd.path)
	if err != nil {
		log.Printf("ERROR: Failed to load root file: %s", err)
		return subcommands.ExitFailure
	}

	c, err := r.GetConversation(cmd.conversationID)
	if err != nil {
		log.Printf("Could not get conversation %q: %s", cmd.conversationID, err)
		return subcommands.ExitFailure
	}

	err = withBufferedWriter(cmd.out, func(w io.Writer) error {
		var excluded int64
		for i := 0; i < c.EventsSize(); i++ {
			e, err := c.Event(i)
			if err != nil {
				log.Printf("Failed to get event #%d: %s", i, err)
				continue
			}

			if cmd.chatID != "" && e.SenderID.ChatID != cmd.chatID {
				continue
			}

			for _, word := range e.AllWords() {
				if excludeRegexp != nil && excludeRegexp.MatchString(word) {
					excluded++
					continue
				}

				// Remove non-alphanumeric characters.
				word = alphaNumeric.ReplaceAllString(word, "")

				if _, err := w.Write([]byte(word)); err != nil {
					return err
				}
				if _, err := w.Write([]byte("\n")); err != nil {
					return err
				}
			}
		}
		log.Printf("Excluded %d word(s)", excluded)
		return nil
	})
	if err != nil {
		log.Printf("Could not write output file: %s", err)
		return subcommands.ExitFailure
	}
	return subcommands.ExitSuccess
}

func (cmd *printAllText) buildExcludeRegexp() (*regexp.Regexp, error) {
	if cmd.excludeRegexpsPath == "" {
		return nil, nil
	}

	fd, err := os.Open(cmd.excludeRegexpsPath)
	if err != nil {
		return nil, err
	}
	defer fd.Close()

	must := func(_ int, err error) {
		if err != nil {
			panic(err)
		}
	}

	var composite strings.Builder
	must(composite.WriteString("(?i)("))
	scanner := bufio.NewScanner(fd)
	count := 0
	for scanner.Scan() {
		if _, err := regexp.Compile(scanner.Text()); err != nil {
			return nil, fmt.Errorf("invalid regexp %q: %w", scanner.Text(), err)
		}
		if count > 0 {
			must(composite.WriteRune('|'))
		}
		must(composite.WriteRune('^'))
		must(composite.WriteString(scanner.Text()))
		must(composite.WriteRune('$'))
		count++
	}
	must(composite.WriteRune(')'))

	log.Printf("Compiling exclude regexp from:\n%s", composite.String())
	re, err := regexp.Compile(composite.String())
	if err != nil {
		return nil, fmt.Errorf("could not build composite regexp: %w", err)
	}
	return re, nil
}

// Main is the entry point to the analysis tool.
func Main(argv []string) int {
	subcommands.Register(subcommands.HelpCommand(), "")
	subcommands.Register(subcommands.FlagsCommand(), "")
	subcommands.Register(subcommands.CommandsCommand(), "")
	subcommands.Register(&listChatsCommand{}, "")
	subcommands.Register(&dumpChatCommand{}, "")
	subcommands.Register(&donwloadAttachmentsCommand{}, "")
	subcommands.Register(&generateUserList{}, "")
	subcommands.Register(&generateBulkImport{}, "")
	subcommands.Register(&printAllText{}, "")

	flag.Parse()
	ctx := context.Background()
	return int(subcommands.Execute(ctx))
}
