package parse

import (
	"context"
	"fmt"
	"io"
	"log"
	"math"
	"mime"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/danjacques/hangouts-migrate/attachment"
	"github.com/hashicorp/go-retryablehttp"
)

func downloadURLSForEmbedItem(ei *EmbedItem) (urls []string) {
	// Use the thumbnail image URL, as this is the only link that actually
	// includes movie downloads.
	if pp := ei.PlusPhoto; pp != nil {
		if t := pp.Thumbnail; t != nil {
			if u := t.URL; u != "" {
				urls = append(urls, u)
			}
			if u := t.ImageURL; u != "" {
				urls = append(urls, u)
			}
		}
		// Fallback to the PlusPhoto URL.
		if u := pp.URL; u != "" {
			urls = append(urls, u)
		}
	}

	// If it's a thing, does it have an image URL?
	if t := ei.ThingV2; t != nil {
		if ri := t.RepresentativeImage; ri != nil {
			if io := ri.ImageObjectV2; io != nil {
				if u := io.URL; u != "" {
					urls = append(urls, u)
				}
			}
		}
	}

	return
}

type ImageDownloader struct {
	AttachmentMapper *attachment.Mapper
	Concurrency      int
	Cookies          []*http.Cookie

	initOnce sync.Once
	sem      semaphore
	client   *retryablehttp.Client
}

type work struct {
	url url.URL
}

func (d *ImageDownloader) initialize() {
	d.initOnce.Do(func() {
		d.sem = make(semaphore, d.Concurrency)

		d.client = retryablehttp.NewClient()
		d.client.Backoff = retryablehttp.DefaultBackoff
		d.client.RetryWaitMin = time.Second * 5
		d.client.RetryWaitMax = time.Minute * 1
		d.client.RetryMax = math.MaxInt32
		d.client.CheckRetry = retryPolicy
	})
}

func (d *ImageDownloader) Add(ei *EmbedItem) bool {
	d.initialize()

	switch path, err := d.AttachmentMapper.ScanPathForKey(ei.Key()); err {
	case nil:
		log.Printf("INFO: File for %s already exists, skipping: %s", ei.Key(), path)
		return false
	case attachment.NotFound:
		break
	default:
		log.Printf("ERROR: Could not scan for key %s, downloading anyway: %s", ei.Key(), err)
	}

	urls := downloadURLSForEmbedItem(ei)
	if len(urls) == 0 {
		// No download URL.
		log.Printf("WARN: Don't know how to get URL for:\n%+v", ei)
		return false
	}

	d.sem.Acquire()
	go func() {
		defer d.sem.Release()
		d.downloadURL(ei.Key(), urls)
	}()
	return true
}

func (d *ImageDownloader) Wait() {
	d.sem.Wait()
}

func (d *ImageDownloader) downloadURL(key string, urls []string) {
	for i, u := range urls {
		if err := d.tryDownloadURL(key, u); err != nil {
			log.Printf("Failed to download key #%d %q at: %s: %s", i, key, u, err)
			continue
		}
		return
	}
	log.Printf("unable to download meaningful content for key %s, tried: %v", key, urls)
}

func (d *ImageDownloader) tryDownloadURL(key, u string) error {
	req, err := retryablehttp.NewRequest("GET", u, nil)
	if err != nil {
		return fmt.Errorf("could not create request: %w", err)
	}
	for _, cookie := range d.Cookies {
		req.AddCookie(cookie)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		log.Printf("Could not download key %q, URL %q: %s", key, u, err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed, non-OK status code %d: %s", resp.StatusCode, resp.Status)
	}

	// Open a writer for |key|. This also does an atomicity check to make sure
	// we don't download the same key more than once.
	mediaType := getMediaType(resp)
	if mediaType == "text/html" {
		// No attachments should be HTML, this is likely an error page.
		return fmt.Errorf("got media type %q, probably error page", mediaType)
	}

	w, err := d.AttachmentMapper.OpenWriter(key, mediaType)
	if err == attachment.Exists {
		log.Printf("An attachment already exists for %q, skipping.", key)
		return nil
	} else if err != nil {
		log.Printf("ERROR: Failed to create attachment writer for %q: %s", key, err)
		return fmt.Errorf("could not open writer for %q: %w", key, err)
	}
	defer func() {
		if w != nil {
			w.Close()
		}
	}()

	const blockSize = 4 * 1024 * 1024
	buf := make([]byte, blockSize)
	written, err := io.CopyBuffer(w, resp.Body, buf)
	if err != nil && err != io.EOF {
		log.Printf("Could not write file for %s: %s", key, err)
		return err
	}

	// Close FD, we care about the error here.
	if err := w.Close(); err != nil {
		log.Printf("Could not close writer for %s: %s", key, err)
		return err
	}

	log.Printf("Successfully downloaded %s (%s) (%d byte(s)) from: %s\nto: %s", key, mediaType, written, u, w.Path())
	w = nil // Do not close in defer.

	return nil
}

// Augment defualt retry policy w/ TooManyRequests.
func retryPolicy(ctx context.Context, resp *http.Response, err error) (bool, error) {
	if retry, err := retryablehttp.DefaultRetryPolicy(ctx, resp, err); retry || err != nil {
		return retry, err
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		return true, nil
	}

	return false, nil
}

func getMediaType(resp *http.Response) string {
	if contentType := resp.Header.Get("Content-Type"); contentType != "" {
		if mediaType, _, err := mime.ParseMediaType(contentType); err == nil {
			return mediaType
		}
		return contentType
	}
	return ""
}
