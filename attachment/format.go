package attachment

type outputFormat struct {
	// Map of source URL to destination file path.
	Entries map[string]string `json:"entries"`
}
