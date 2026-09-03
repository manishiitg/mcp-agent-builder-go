// Package commonsimages finds an openly licensed picture on Wikimedia
// Commons for a topic and downloads a thumbnail of it. Shared by the
// SparkQuill family server and the SparkQuill platform product; nothing
// here knows where the picture will be saved.
package commonsimages

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Pictures are fetched from Wikimedia Commons rather than a general image
// search, and that choice is load-bearing rather than incidental:
//
//   - Licensing. A progress report or academic map can be PUBLISHED to a static
//     host or Drive for a co-parent or tutor (see skills/publish). Commons media
//     is openly licensed and carries its attribution in the API response, so a
//     published page is fine. Arbitrary search-engine results would not be.
//   - Safety. This lands on a child's page. Commons is human-curated; a general
//     image search is not, and nothing here has a human in the loop.
//   - Reliability. Commons has a real JSON API. Image-search pages lazy-load
//     base64 thumbnails behind anti-automation measures, so scraping one through
//     the browser connector would be flaky in exactly the way that wastes a turn.
//   - Coverage. Commons is strongest on precisely the school subjects this app
//     deals with: geography, maps, biology, history, astronomy, diagrams.
//
// Wikimedia asks API clients to identify themselves; an anonymous or generic
// agent is rate-limited or blocked outright.
// UserAgent identifies these requests to Wikimedia, as their policy asks.
const UserAgent = "SparkQuill/1.0 (family learning app; local, non-commercial)"

// commonsThumbWidth is the width we ask Commons to scale to. Full-size originals
// run to many megabytes, which would bloat the workspace for no visible gain —
// these are read on one pane of a laptop screen.
const commonsThumbWidth = 900

// MaxImageBytes bounds a single download so an unexpectedly large file can't
// fill the workspace. Comfortably above a 900px-wide photo.
// MaxImageBytes caps a downloaded thumbnail.
const MaxImageBytes = 6 << 20 // 6 MiB

// Image is one usable picture found on Commons.
type Image struct {
	Title    string
	ThumbURL string
	PageURL  string
	License  string
	Artist   string
	Width    int
	Height   int
	Mime     string
}

// htmlTagRE strips the markup Commons returns inside extmetadata fields (Artist
// arrives as an <a> tag), leaving plain text fit for a caption.
var htmlTagRE = regexp.MustCompile(`<[^>]*>`)

// commonsInfo is one imageinfo entry, shared by both lookup paths (search and
// by-title) so the picture/license parsing exists once.
type commonsInfo struct {
	ThumbURL       string `json:"thumburl"`
	DescriptionURL string `json:"descriptionurl"`
	Mime           string `json:"mime"`
	ThumbWidth     int    `json:"thumbwidth"`
	ThumbHeight    int    `json:"thumbheight"`
	// Value is deliberately not a string: Commons returns numbers here for some
	// fields (confirmed live — decoding as string failed the whole response and
	// lost the picture entirely).
	ExtMetadata map[string]struct {
		Value interface{} `json:"value"`
	} `json:"extmetadata"`
}

// toImage converts an imageinfo entry into a usable picture, or nil when it
// isn't one. Commons' File: namespace also holds audio, video and scanned PDFs,
// none of which belong in an <img>.
func (c commonsInfo) toImage(pageTitle string) *Image {
	if !strings.HasPrefix(c.Mime, "image/") || c.ThumbURL == "" {
		return nil
	}
	meta := func(k string) string {
		v, ok := c.ExtMetadata[k]
		if !ok || v.Value == nil {
			return ""
		}
		return strings.TrimSpace(htmlTagRE.ReplaceAllString(fmt.Sprint(v.Value), ""))
	}
	return &Image{
		Title:    strings.TrimPrefix(pageTitle, "File:"),
		ThumbURL: c.ThumbURL,
		PageURL:  c.DescriptionURL,
		License:  meta("LicenseShortName"),
		Artist:   meta("Artist"),
		Width:    c.ThumbWidth,
		Height:   c.ThumbHeight,
		Mime:     c.Mime,
	}
}

// commonsFileTypeFilter restricts results to actual pictures at the SEARCH
// layer, and it is the difference between this tool working and not. Commons'
// File: namespace is full of digitized documents, and full-text search ranks
// them highly for anything science-flavored: "solar radiation angle latitude"
// came back as eight scanned PDFs of 1920s research papers and zero images, so
// the tool honestly reported "nothing found" while Commons held plenty of
// perfectly good diagrams. Filtering server-side turned that same query into
// eight usable images.
const commonsFileTypeFilter = " filetype:bitmap|drawing"

// searchCommons finds the best picture for query, or nil when there is nothing
// suitable — a normal outcome, not an error.
//
// Two strategies, in this order, because they fail in opposite situations:
//
//  1. The WIKIPEDIA ARTICLE on the topic, and the image its editors chose to
//     illustrate it. This is the good path. Article search understands a
//     natural-language topic, and a human has already picked the clearest
//     diagram for exactly this concept.
//  2. Commons FULL-TEXT FILE search, as a fallback for subjects with no article
//     (a specific local landmark, a named worksheet figure).
//
// Article-first exists because file search demands that every word appear in a
// file's own text, so the descriptive phrasing a tutor naturally writes matches
// nothing: "Earth seasons solstice sun Tropic of Cancer diagram" found no file at
// all, while the same words led to the article "Sun path" and its
// Solar_altitude.svg — the exactly-right picture. Blindly shortening the query
// instead was tried and rejected: "Earth sun rays" returned a photograph of the
// full moon, and a confidently irrelevant picture on a child's page is worse than
// none.
// Search finds the best picture for a short subject query; nil when
// nothing usable exists.
func Search(ctx context.Context, query string) (*Image, error) {
	if img, err := searchViaWikipediaArticle(ctx, query); err == nil && img != nil {
		return img, nil
	}
	return searchCommonsFiles(ctx, query)
}

// wikipediaChromeRE matches the furniture that appears in article image lists —
// maintenance banners, project logos, UI icons — none of which is ever the
// picture a child should be shown.
var wikipediaChromeRE = regexp.MustCompile(`(?i)(question[_ ]book|commons-logo|wiki[a-z]*-logo|wikimedia|wiktionary|wikisource|wikiquote|ambox|imbox|ombox|edit-clear|symbol_|disambig|padlock|portal|folder_hexagonal|people_icon|red_pencil|magnify-clip|increase|decrease|blank\.|\.ogg|\.oga|\.wav|\.mid|\.webm|\.ogv)`)

// numberGroupRE finds runs of digits in a filename. Several of them usually mean
// the file is one plot from a generated series ("SPC Polar -51.92 0.0.png"),
// which illustrates one parameter value rather than the idea being taught.
var numberGroupRE = regexp.MustCompile(`\d+`)

// stopWords are the words too common to signal that a filename is on-topic.
var stopWords = map[string]bool{
	"the": true, "and": true, "for": true, "with": true, "from": true, "into": true,
	"diagram": true, "image": true, "picture": true, "photo": true, "file": true,
	"png": true, "jpg": true, "jpeg": true, "svg": true, "gif": true,
}

// significantWords lowercases text and keeps the words worth matching on.
func significantWords(text string) map[string]bool {
	out := map[string]bool{}
	for _, w := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9')
	}) {
		if len(w) > 2 && !stopWords[w] {
			out[w] = true
		}
	}
	return out
}

// scoreArticleImage ranks one article image by how likely it is to be the
// picture that explains the topic, rather than incidental page furniture.
func scoreArticleImage(title string, topicWords map[string]bool) int {
	score := 0
	for w := range significantWords(title) {
		if topicWords[w] {
			score += 2 // names the subject
		}
	}
	if strings.HasSuffix(strings.ToLower(title), ".svg") {
		score++ // drawn diagrams are usually the teaching illustration
	}
	if len(numberGroupRE.FindAllString(title, -1)) >= 2 {
		score -= 3 // one plot out of a parameter sweep
	}
	return score
}

// searchViaWikipediaArticle finds the article for a topic and returns the image
// that best illustrates it: the lead image (pageimages picks the article's
// representative one) when there is one, else the first non-chrome image on the
// page. Both are Commons files, so licensing and attribution work identically.
func searchViaWikipediaArticle(ctx context.Context, query string) (*Image, error) {
	endpoint := "https://en.wikipedia.org/w/api.php?" + url.Values{
		"action":       {"query"},
		"format":       {"json"},
		"generator":    {"search"},
		"gsrsearch":    {query},
		"gsrnamespace": {"0"}, // articles, not files
		"gsrlimit":     {"1"},
		"prop":         {"pageimages|images"},
		"piprop":       {"original"},
		"imlimit":      {"25"},
	}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("wikipedia returned %s", resp.Status)
	}
	var payload struct {
		Query struct {
			Pages map[string]struct {
				Title    string `json:"title"`
				Original struct {
					Source string `json:"source"`
				} `json:"original"`
				Images []struct {
					Title string `json:"title"`
				} `json:"images"`
			} `json:"pages"`
		} `json:"query"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&payload); err != nil {
		return nil, err
	}

	for _, page := range payload.Query.Pages {
		var candidates []string
		// The lead image, recovered as a File: title from its upload URL so it
		// goes through the same Commons metadata lookup as anything else.
		if src := page.Original.Source; src != "" {
			if name, err := url.PathUnescape(src[strings.LastIndex(src, "/")+1:]); err == nil && name != "" {
				candidates = append(candidates, "File:"+name)
			}
		}
		// No curated lead image on this article, so fall back to its image list —
		// but RANKED, not in the order the API happens to return it. That order is
		// alphabetical and therefore meaningless: for "Sun path" it put
		// "SPC Polar -51.92 0.0.png" (a polar chart plotted for one specific
		// latitude) ahead of "Solar altitude.svg", the actual explanatory diagram.
		var rest []string
		for _, im := range page.Images {
			if !wikipediaChromeRE.MatchString(im.Title) {
				rest = append(rest, im.Title)
			}
		}
		topicWords := significantWords(query + " " + page.Title)
		sort.SliceStable(rest, func(i, j int) bool {
			return scoreArticleImage(rest[i], topicWords) > scoreArticleImage(rest[j], topicWords)
		})
		candidates = append(candidates, rest...)
		for _, title := range candidates {
			img, err := ImageByTitle(ctx, title)
			if err == nil && img != nil {
				log.Printf("[find_image] %q -> wikipedia article %q -> %s", query, page.Title, img.Title)
				return img, nil
			}
		}
	}
	return nil, nil
}

// ImageByTitle pulls the thumbnail URL and license metadata for one known
// Commons file title. Returns nil (not an error) when the title isn't a usable
// picture on Commons — which correctly skips files hosted locally on Wikipedia
// under fair use rather than freely licensed on Commons.
func ImageByTitle(ctx context.Context, title string) (*Image, error) {
	endpoint := "https://commons.wikimedia.org/w/api.php?" + url.Values{
		"action":     {"query"},
		"format":     {"json"},
		"titles":     {title},
		"prop":       {"imageinfo"},
		"iiprop":     {"url|extmetadata|size|mime"},
		"iiurlwidth": {fmt.Sprint(commonsThumbWidth)},
	}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("commons returned %s", resp.Status)
	}
	var payload struct {
		Query struct {
			Pages map[string]struct {
				Title     string          `json:"title"`
				Missing   json.RawMessage `json:"missing"`
				ImageInfo []commonsInfo   `json:"imageinfo"`
			} `json:"pages"`
		} `json:"query"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&payload); err != nil {
		return nil, err
	}
	for _, page := range payload.Query.Pages {
		if len(page.ImageInfo) == 0 {
			continue
		}
		if img := page.ImageInfo[0].toImage(page.Title); img != nil {
			return img, nil
		}
	}
	return nil, nil
}

func searchCommonsFiles(ctx context.Context, query string) (*Image, error) {
	endpoint := "https://commons.wikimedia.org/w/api.php?" + url.Values{
		"action":       {"query"},
		"format":       {"json"},
		"generator":    {"search"},
		"gsrsearch":    {query + commonsFileTypeFilter},
		"gsrnamespace": {"6"}, // File: namespace only
		"gsrlimit":     {"8"},
		"prop":         {"imageinfo"},
		"iiprop":       {"url|extmetadata|size|mime"},
		"iiurlwidth":   {fmt.Sprint(commonsThumbWidth)},
	}.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("could not reach Wikimedia Commons: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Wikimedia Commons returned %s", resp.Status)
	}

	var payload struct {
		Query struct {
			Pages map[string]struct {
				Title string `json:"title"`
				// Index is the search RANK (1 = best). It has to be read and
				// sorted on: pages arrives as a JSON object, and Go map
				// iteration is randomized, so taking "the first usable page"
				// silently returned an arbitrary one of the eight results
				// instead of the best — erratic picture quality that would have
				// looked like the search itself being bad.
				Index     int           `json:"index"`
				ImageInfo []commonsInfo `json:"imageinfo"`
			} `json:"pages"`
		} `json:"query"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("could not read the Commons response: %w", err)
	}

	// Best-ranked first, since map order is meaningless (see Index above).
	ranked := make([]int, 0, len(payload.Query.Pages))
	byIndex := map[int]string{}
	for key, page := range payload.Query.Pages {
		ranked = append(ranked, page.Index)
		byIndex[page.Index] = key
	}
	sort.Ints(ranked)

	for _, idx := range ranked {
		page := payload.Query.Pages[byIndex[idx]]
		if len(page.ImageInfo) == 0 {
			continue
		}
		if img := page.ImageInfo[0].toImage(page.Title); img != nil {
			return img, nil
		}
	}
	return nil, nil // searched fine, nothing usable — the caller words this for the reader
}

// downloadCommonsImage saves img's scaled thumbnail into absDir and returns the
// bare filename to reference from the page.
// Download fetches the thumbnail and returns its bytes with the file
// extension that matches its type.
func Download(ctx context.Context, img *Image) (data []byte, ext string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, img.ThumbURL, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", UserAgent)
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("could not download the picture: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("downloading the picture returned %s", resp.Status)
	}
	data, err = io.ReadAll(io.LimitReader(resp.Body, MaxImageBytes+1))
	if err != nil {
		return nil, "", fmt.Errorf("could not download the picture: %w", err)
	}
	if len(data) > MaxImageBytes {
		return nil, "", fmt.Errorf("that picture is unexpectedly large; skipped")
	}
	ext = ".jpg"
	switch {
	case strings.Contains(img.Mime, "png"), strings.HasSuffix(strings.ToLower(img.ThumbURL), ".png"):
		ext = ".png"
	case strings.Contains(img.Mime, "gif"):
		ext = ".gif"
	case strings.Contains(img.Mime, "webp"):
		ext = ".webp"
	}
	return data, ext, nil
}

// FileName builds the saved filename from the caller's slug and the
// downloaded type.
func FileName(slug, ext string) string {
	name := strings.ToLower(strings.TrimSpace(slug))
	name = strings.NewReplacer("/", "_", "\\", "_", "..", "_", " ", "_").Replace(name)
	if i := strings.LastIndex(name, "."); i > 0 {
		name = name[:i]
	}
	if name == "" || name == "." {
		name = "picture"
	}
	return name + ext
}

// Credit is the attribution line to print under the picture.
func Credit(img *Image) string {
	credit := strings.TrimSpace(strings.Join([]string{img.Artist, img.License}, " · "))
	return strings.Trim(credit, " ·")
}

// Params is the tool parameter schema shared by every find_image tool.
var Params = map[string]interface{}{
	"type": "object",
	"properties": map[string]interface{}{
		"query": map[string]interface{}{
			"type": "string",
			"description": "SHORT subject terms — 2 to 4 words naming the thing itself, like a caption: \"latitude parallels world map\", " +
				"\"plateau landform\", \"digestive system diagram\". Not a sentence and not the lesson's wording: a long descriptive " +
				"phrase matches nothing (all terms must appear), so \"Earth sun rays angle latitude climate diagram\" finds nothing " +
				"while \"sunlight angle Earth\" finds the right diagram. If you get no_match, retry once with fewer, plainer nouns.",
		},
		"filename": map[string]interface{}{
			"type":        "string",
			"description": "short slug for the saved file, e.g. \"latitude-map\" (no extension)",
		},
		"dir": map[string]interface{}{
			"type":        "string",
			"description": "workspace-relative folder to save into (the activity folder, or reports/). Parent Mode only; ignored in Child Mode.",
		},
	},
	"required": []string{"query", "filename"},
}

// Description is the tool description shared by every find_image tool.
const Description = "Find a real photo, map or diagram for a topic and save it next to the page you're writing, " +
	"so the child SEES the thing instead of only reading about it. Pictures come from Wikimedia Commons — openly " +
	"licensed, so they're safe to include in something the parent may share. Returns the saved filename plus the " +
	"attribution to print under it. Reference it with a plain relative tag: <img src=\"FILENAME\" alt=\"...\">. " +
	"Use it where a picture genuinely teaches (a map for latitude, a labeled diagram for a body system, a real " +
	"photo of a landform) — for simple charts, shapes and arrows an inline SVG is still better. If nothing suitable " +
	"is found, say so and carry on without a picture; never invent a filename you did not get back from this tool."

	// findImageTool builds the shared find_image tool. resolveDir turns the model's
	// requested dir into an absolute path it is actually allowed to write to, which
	// is where Parent and Child Mode differ: the parent may place a picture beside
	// any page it is authoring, while the child is pinned to its one activity
	// folder no matter what it passes.
