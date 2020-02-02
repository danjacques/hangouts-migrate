package attachment

import (
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
)

type Writer struct {
	destPath string

	tempFile *os.File
}

func makeWriter(destPath string) (*Writer, error) {
	w := &Writer{
		destPath: destPath,
	}

	// Create our temporary file.
	var err error
	w.tempFile, err = ioutil.TempFile(filepath.Dir(w.destPath), "tmp-"+filepath.Base(w.destPath))
	if err != nil {
		return nil, err
	}
	return w, nil
}

func (w *Writer) Path() string { return w.destPath }

func (w *Writer) Write(data []byte) (int, error) {
	return w.tempFile.Write(data)
}

func (w *Writer) Close() error {
	tempFileName := w.tempFile.Name()
	defer func() {
		if tempFileName != "" {
			if err := os.Remove(tempFileName); err != nil {
				log.Printf("WARNING: Failed to remove temporary file %s: %s", tempFileName, err)
			}
		}
	}()

	// Close the temporary file.
	if err := w.tempFile.Close(); err != nil {
		return err
	}

	// Rename it to our destination file.
	if err := os.Rename(tempFileName, w.destPath); err != nil {
		return err
	}

	// Clear our tempFileName, marking that no delete needs to happen in defer.
	tempFileName = ""
	return nil
}
