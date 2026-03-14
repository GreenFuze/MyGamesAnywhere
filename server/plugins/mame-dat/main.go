package main

import (
	"archive/zip"
	"encoding/binary"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// IPC protocol types.

type Request struct {
	ID     string          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

type Response struct {
	ID     string `json:"id"`
	Result any    `json:"result,omitempty"`
	Error  *Error `json:"error,omitempty"`
}

type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Index entry: the compact representation we persist and keep in memory.
type indexEntry struct {
	Name         string `json:"name"`
	CloneOf      string `json:"clone_of,omitempty"`
	Description  string `json:"description"`
	Year         string `json:"year,omitempty"`
	Manufacturer string `json:"manufacturer,omitempty"`
}

type mameLookup struct {
	index map[string]*indexEntry
}

func (l *mameLookup) lookup(name string) *indexEntry {
	return l.index[strings.ToLower(name)]
}

// Plugin request/response types.

type lookupParams struct {
	Games  []gameQuery    `json:"games"`
	Config map[string]any `json:"config"`
}

type gameQuery struct {
	Index     int    `json:"index"`
	Title     string `json:"title"`
	Platform  string `json:"platform"`
	RootPath  string `json:"root_path"`
	GroupKind string `json:"group_kind"`
}

type lookupResult struct {
	Index      int    `json:"index"`
	Title      string `json:"title,omitempty"`
	Platform   string `json:"platform,omitempty"`
	ExternalID string `json:"external_id"`
	URL        string `json:"url,omitempty"`
}

// GitHub API types (subset).

type ghRelease struct {
	TagName string    `json:"tag_name"`
	Assets  []ghAsset `json:"assets"`
}

type ghAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// Paths relative to the plugin's working directory.
const (
	dataDir      = "data"
	indexFile    = "data/index.json"
	versionFile  = "data/.version"
	releaseAPI   = "https://api.github.com/repos/mamedev/mame/releases/latest"
	httpTimeout  = 5 * time.Minute
)

var cache *mameLookup

// --- Init: ensure data is present ---

func handleInit() (any, *Error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, &Error{Code: "INIT_FAILED", Message: "create data dir: " + err.Error()}
	}

	localVer := readVersionFile()
	remoteVer, downloadURL, err := fetchLatestRelease()
	if err != nil {
		log.Printf("WARNING: could not fetch latest release: %v", err)
		if localVer != "" {
			log.Printf("using cached index (version %s)", localVer)
			return map[string]any{"status": "ok", "version": localVer, "cached": true}, nil
		}
		return nil, &Error{Code: "INIT_FAILED", Message: "no cached data and cannot fetch latest: " + err.Error()}
	}

	if localVer == remoteVer {
		log.Printf("index is up to date (version %s)", localVer)
		return map[string]any{"status": "ok", "version": localVer}, nil
	}

	log.Printf("updating index: %s -> %s", localVer, remoteVer)
	if err := downloadAndBuildIndex(downloadURL, remoteVer); err != nil {
		log.Printf("WARNING: update failed: %v", err)
		if localVer != "" {
			log.Printf("falling back to cached index (version %s)", localVer)
			return map[string]any{"status": "ok", "version": localVer, "cached": true}, nil
		}
		return nil, &Error{Code: "INIT_FAILED", Message: err.Error()}
	}

	cache = nil
	return map[string]any{"status": "ok", "version": remoteVer}, nil
}

func readVersionFile() string {
	data, err := os.ReadFile(versionFile)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func fetchLatestRelease() (tag string, downloadURL string, err error) {
	client := &http.Client{Timeout: 30 * time.Second}
	req, _ := http.NewRequest("GET", releaseAPI, nil)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("GET %s: %w", releaseAPI, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", "", fmt.Errorf("GET %s: status %d", releaseAPI, resp.StatusCode)
	}

	var rel ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", "", fmt.Errorf("decode release JSON: %w", err)
	}

	for _, a := range rel.Assets {
		if strings.HasSuffix(a.Name, "lx.zip") {
			return rel.TagName, a.BrowserDownloadURL, nil
		}
	}
	return "", "", fmt.Errorf("no *lx.zip asset found in release %s", rel.TagName)
}

func downloadAndBuildIndex(url, version string) error {
	tmpFile, err := os.CreateTemp("", "mame-dat-*.zip")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	log.Printf("downloading %s", url)
	client := &http.Client{Timeout: httpTimeout}
	resp, err := client.Get(url)
	if err != nil {
		tmpFile.Close()
		return fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		tmpFile.Close()
		return fmt.Errorf("download: status %d", resp.StatusCode)
	}

	written, err := io.Copy(tmpFile, resp.Body)
	if err != nil {
		tmpFile.Close()
		return fmt.Errorf("write zip: %w", err)
	}
	tmpFile.Close()
	log.Printf("downloaded %d bytes", written)

	zr, err := zip.OpenReader(tmpPath)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer zr.Close()

	xmlEntry, err := findXMLEntry(zr, version)
	if err != nil {
		return err
	}

	rc, err := xmlEntry.Open()
	if err != nil {
		return fmt.Errorf("open zip entry: %w", err)
	}
	defer rc.Close()

	log.Printf("stream-parsing %s (%d bytes uncompressed)", xmlEntry.Name, xmlEntry.UncompressedSize64)
	entries, err := streamParseMAME(rc)
	if err != nil {
		return fmt.Errorf("parse XML: %w", err)
	}
	log.Printf("parsed %d machines", len(entries))

	data, err := json.Marshal(entries)
	if err != nil {
		return fmt.Errorf("marshal index: %w", err)
	}
	if err := os.WriteFile(indexFile, data, 0o644); err != nil {
		return fmt.Errorf("write index: %w", err)
	}
	if err := os.WriteFile(versionFile, []byte(version), 0o644); err != nil {
		return fmt.Errorf("write version: %w", err)
	}
	log.Printf("wrote index (%d bytes) for version %s", len(data), version)
	return nil
}

func findXMLEntry(zr *zip.ReadCloser, version string) (*zip.File, error) {
	var xmlFiles []*zip.File
	for _, f := range zr.File {
		if strings.HasSuffix(strings.ToLower(f.Name), ".xml") && !strings.Contains(f.Name, "/") {
			xmlFiles = append(xmlFiles, f)
		}
	}

	if len(xmlFiles) == 1 {
		return xmlFiles[0], nil
	}

	tag := strings.TrimPrefix(version, "mame")
	for _, f := range xmlFiles {
		if strings.EqualFold(f.Name, "mame"+tag+".xml") {
			return f, nil
		}
	}
	for _, f := range xmlFiles {
		if strings.EqualFold(f.Name, "mame.xml") {
			return f, nil
		}
	}

	names := make([]string, len(xmlFiles))
	for i, f := range xmlFiles {
		names[i] = f.Name
	}
	return nil, fmt.Errorf("could not find XML in zip (found: %v)", names)
}

// streamParseMAME reads a MAME listxml stream, extracting only the fields we need.
// Handles both <mame> and <datafile> root elements.
func streamParseMAME(r io.Reader) ([]indexEntry, error) {
	dec := xml.NewDecoder(r)
	var entries []indexEntry

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}

		if se.Name.Local == "machine" || se.Name.Local == "game" {
			entry := parseMachineElement(dec, se)
			entries = append(entries, entry)
		}
	}
	return entries, nil
}

func parseMachineElement(dec *xml.Decoder, start xml.StartElement) indexEntry {
	var entry indexEntry
	for _, attr := range start.Attr {
		switch attr.Name.Local {
		case "name":
			entry.Name = attr.Value
		case "cloneof":
			entry.CloneOf = attr.Value
		}
	}

	depth := 1
	for depth > 0 {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			switch t.Name.Local {
			case "description":
				entry.Description = readElementText(dec, &depth)
			case "year":
				entry.Year = readElementText(dec, &depth)
			case "manufacturer":
				entry.Manufacturer = readElementText(dec, &depth)
			}
		case xml.EndElement:
			depth--
		}
	}
	return entry
}

func readElementText(dec *xml.Decoder, depth *int) string {
	var sb strings.Builder
	for {
		tok, err := dec.Token()
		if err != nil {
			return sb.String()
		}
		switch t := tok.(type) {
		case xml.CharData:
			sb.Write(t)
		case xml.EndElement:
			*depth--
			return sb.String()
		case xml.StartElement:
			*depth++
		}
	}
}

// --- Index loading ---

func loadIndex() (*mameLookup, error) {
	data, err := os.ReadFile(indexFile)
	if err != nil {
		return nil, fmt.Errorf("read index: %w", err)
	}

	var entries []indexEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("unmarshal index: %w", err)
	}

	idx := make(map[string]*indexEntry, len(entries))
	for i := range entries {
		e := &entries[i]
		idx[strings.ToLower(e.Name)] = e
	}
	return &mameLookup{index: idx}, nil
}

func getOrLoadIndex() (*mameLookup, error) {
	if cache != nil {
		return cache, nil
	}
	lk, err := loadIndex()
	if err != nil {
		return nil, err
	}
	cache = lk
	log.Printf("loaded index: %d machines", len(lk.index))
	return lk, nil
}

// --- Lookup ---

func handleLookup(params lookupParams) (any, *Error) {
	dat, err := getOrLoadIndex()
	if err != nil {
		return nil, &Error{Code: "INDEX_NOT_READY", Message: err.Error()}
	}

	var results []lookupResult
	for _, q := range params.Games {
		name := strings.TrimSuffix(q.Title, filepath.Ext(q.Title))

		m := dat.lookup(name)
		if m == nil {
			continue
		}

		r := lookupResult{
			Index:      q.Index,
			Title:      m.Description,
			Platform:   "arcade",
			ExternalID: m.Name,
			URL:        "https://www.arcade-museum.com/Machine/" + m.Name,
		}
		results = append(results, r)
	}

	return map[string]any{"results": results}, nil
}

// --- Main loop ---

func main() {
	log.SetOutput(os.Stderr)
	log.Println("MAME DAT metadata plugin started")

	for {
		var length uint32
		if err := binary.Read(os.Stdin, binary.BigEndian, &length); err != nil {
			if err == io.EOF {
				return
			}
			log.Fatalf("read length: %v", err)
		}

		payload := make([]byte, length)
		if _, err := io.ReadFull(os.Stdin, payload); err != nil {
			log.Fatalf("read payload: %v", err)
		}

		var req Request
		if err := json.Unmarshal(payload, &req); err != nil {
			log.Printf("unmarshal request: %v", err)
			continue
		}

		var resp Response
		resp.ID = req.ID

		switch req.Method {
		case "plugin.init":
			result, errObj := handleInit()
			if errObj != nil {
				resp.Error = errObj
			} else {
				resp.Result = result
			}

		case "plugin.info":
			resp.Result = map[string]any{
				"plugin_id":      "metadata-mame-dat",
				"plugin_version": "1.0.0",
				"capabilities":   []string{"metadata"},
			}

		case "plugin.check_config":
			resp.Result = map[string]any{"status": "ok"}

		case "metadata.game.lookup":
			var params lookupParams
			if err := json.Unmarshal(req.Params, &params); err != nil {
				resp.Error = &Error{Code: "INVALID_PARAMS", Message: err.Error()}
			} else {
				result, errObj := handleLookup(params)
				if errObj != nil {
					resp.Error = errObj
				} else {
					resp.Result = result
				}
			}

		default:
			resp.Error = &Error{Code: "UNKNOWN_METHOD", Message: "unknown method: " + req.Method}
		}

		out, _ := json.Marshal(resp)
		binary.Write(os.Stdout, binary.BigEndian, uint32(len(out)))
		os.Stdout.Write(out)
	}
}
