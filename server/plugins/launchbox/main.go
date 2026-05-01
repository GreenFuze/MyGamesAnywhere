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
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/pkg/titlematch"
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

// Index types.

type fileEntry struct {
	Platform string `json:"p"`
	FileName string `json:"f"`
	GameName string `json:"g"`
}

type gameEntry struct {
	DatabaseID int    `json:"i"`
	Name       string `json:"n"`
	Platform   string `json:"p"`
	Year       string `json:"y,omitempty"`
	Developer  string `json:"d,omitempty"`
	Publisher  string `json:"u,omitempty"`
	Genres     string `json:"g,omitempty"`
	Overview   string `json:"o,omitempty"`
	MaxPlayers string `json:"m,omitempty"`
	VideoURL   string `json:"v,omitempty"`
	Rating     string `json:"r,omitempty"`
}

type gameImage struct {
	DatabaseID int    `json:"i"`
	FileName   string `json:"f"`
	Type       string `json:"t"`
}

type tokenedGame struct {
	tokens map[string]bool
	game   *gameEntry
}

type launchBoxIndex struct {
	files      map[string]*fileEntry    // "lower(platform)\tlower(filename)" → entry
	games      map[string]*gameEntry    // "lower(platform)\tlower(name)" → entry
	normalized map[string]*gameEntry    // "lower(platform)\tnormalized(name)" → entry
	byPlatform map[string][]tokenedGame // lower(LB platform) → games with precomputed tokens
	images     map[int][]gameImage      // DatabaseID → images
}

func (idx *launchBoxIndex) lookupFile(platform, filename string) *fileEntry {
	return idx.files[strings.ToLower(platform)+"\t"+strings.ToLower(filename)]
}

func (idx *launchBoxIndex) lookupGame(platform, name string) *gameEntry {
	return idx.games[strings.ToLower(platform)+"\t"+strings.ToLower(name)]
}

func (idx *launchBoxIndex) lookupNormalized(platform, title string) *gameEntry {
	return idx.normalized[strings.ToLower(platform)+"\t"+normalizeTitle(title)]
}

// --- Title normalization & token matching ---

var (
	trailingParensRE = regexp.MustCompile(`[\s_]*\([^)]*\)\s*$`)
	setupPrefixRE    = regexp.MustCompile(`^setup[_\s]`)
	versionSuffixRE  = regexp.MustCompile(`[\s._]+v?\d+(\.\d+)+([\s._]+[a-z]{2,3})*\s*$`)
	nonAlphaNumRE    = regexp.MustCompile(`[^a-z0-9\s]+`)
	multiSpaceRE     = regexp.MustCompile(`\s{2,}`)
)

func normalizeTitle(s string) string {
	return titlematch.NormalizeLookupTitle(s)
}

func tokenize(s string) map[string]bool {
	words := strings.Fields(titlematch.NormalizeLookupTitle(s))
	tokens := make(map[string]bool, len(words))
	for _, w := range words {
		if arabic, ok := romanToArabic[w]; ok {
			w = arabic
		}
		tokens[w] = true
	}
	return tokens
}

const (
	minJaccard        = 0.5
	minMatchingTokens = 2
	maxManualResults  = 10
)

// --- Title variations (roman ↔ arabic numerals) ---

var arabicToRoman = []struct {
	arabic int
	roman  string
}{
	{20, "xx"}, {19, "xix"}, {18, "xviii"}, {17, "xvii"}, {16, "xvi"},
	{15, "xv"}, {14, "xiv"}, {13, "xiii"}, {12, "xii"}, {11, "xi"},
	{10, "x"}, {9, "ix"}, {8, "viii"}, {7, "vii"}, {6, "vi"},
	{5, "v"}, {4, "iv"}, {3, "iii"}, {2, "ii"},
}

var romanToArabic map[string]string

func init() {
	romanToArabic = make(map[string]string, len(arabicToRoman))
	for _, pair := range arabicToRoman {
		romanToArabic[pair.roman] = fmt.Sprintf("%d", pair.arabic)
	}
}

var trailingNumberRE = regexp.MustCompile(`^(.+)\s+(\S+)$`)

func titleVariations(normalized string) []string {
	m := trailingNumberRE.FindStringSubmatch(normalized)
	if m == nil {
		return nil
	}
	base, suffix := m[1], m[2]
	var variations []string

	if arabic, ok := romanToArabic[suffix]; ok {
		variations = append(variations, base+" "+arabic)
		if arabic == "1" {
			variations = append(variations, base)
		}
	}

	if suffix == "1" {
		variations = append(variations, base+" i")
		variations = append(variations, base)
	} else if n := parseSmallInt(suffix); n >= 2 && n <= 20 {
		for _, pair := range arabicToRoman {
			if pair.arabic == n {
				variations = append(variations, base+" "+pair.roman)
				break
			}
		}
	}

	return variations
}

func parseSmallInt(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return -1
		}
		n = n*10 + int(c-'0')
		if n > 20 {
			return -1
		}
	}
	return n
}

func bestTokenMatch(queryTokens map[string]bool, candidates []tokenedGame) *gameEntry {
	var best *gameEntry
	bestScore := 0.0
	for i := range candidates {
		intersection := 0
		for t := range queryTokens {
			if candidates[i].tokens[t] {
				intersection++
			}
		}
		if intersection < minMatchingTokens {
			continue
		}
		union := len(queryTokens) + len(candidates[i].tokens) - intersection
		score := float64(intersection) / float64(union)
		if score > bestScore {
			bestScore = score
			best = candidates[i].game
		}
	}
	if bestScore >= minJaccard {
		return best
	}
	return nil
}

type scoredGameEntry struct {
	game  *gameEntry
	score float64
}

func tokenMatches(queryTokens map[string]bool, candidates []tokenedGame) []scoredGameEntry {
	var matches []scoredGameEntry
	for i := range candidates {
		intersection := 0
		for t := range queryTokens {
			if candidates[i].tokens[t] {
				intersection++
			}
		}
		if intersection < minMatchingTokens {
			continue
		}
		union := len(queryTokens) + len(candidates[i].tokens) - intersection
		score := float64(intersection) / float64(union)
		if score >= minJaccard {
			matches = append(matches, scoredGameEntry{game: candidates[i].game, score: score})
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score > matches[j].score
		}
		return matches[i].game.Name < matches[j].game.Name
	})
	return matches
}

// Plugin request/response types.

type lookupParams struct {
	Games  []gameQuery    `json:"games"`
	Config map[string]any `json:"config"`
}

type gameQuery struct {
	Index        int    `json:"index"`
	Title        string `json:"title"`
	Platform     string `json:"platform"`
	RootPath     string `json:"root_path"`
	GroupKind    string `json:"group_kind"`
	LookupIntent string `json:"lookup_intent,omitempty"`
}

type mediaItemIPC struct {
	Type     string `json:"type"`
	URL      string `json:"url"`
	Width    int    `json:"width,omitempty"`
	Height   int    `json:"height,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
}

type lookupResult struct {
	Index       int            `json:"index"`
	Title       string         `json:"title,omitempty"`
	Platform    string         `json:"platform,omitempty"`
	ExternalID  string         `json:"external_id"`
	URL         string         `json:"url,omitempty"`
	Description string         `json:"description,omitempty"`
	ReleaseDate string         `json:"release_date,omitempty"`
	Genres      []string       `json:"genres,omitempty"`
	Developer   string         `json:"developer,omitempty"`
	Publisher   string         `json:"publisher,omitempty"`
	Media       []mediaItemIPC `json:"media,omitempty"`
	Rating      float64        `json:"rating,omitempty"`
	MaxPlayers  int            `json:"max_players,omitempty"`
}

// LaunchBox provider platform aliases keyed by the canonical platform model.
var launchBoxPlatformAliases = map[core.Platform][]string{
	core.PlatformWindowsPC:        {"Windows"},
	core.PlatformMSDOS:            {"MS-DOS", "Windows"},
	core.PlatformArcade:           {"Arcade"},
	core.PlatformNES:              {"Nintendo Entertainment System"},
	core.PlatformSNES:             {"Super Nintendo Entertainment System"},
	core.PlatformGB:               {"Nintendo Game Boy"},
	core.PlatformGBC:              {"Nintendo Game Boy Color"},
	core.PlatformGBA:              {"Nintendo Game Boy Advance"},
	core.PlatformN64:              {"Nintendo 64"},
	core.PlatformGenesis:          {"Sega Genesis", "Sega Mega Drive"},
	core.PlatformSegaMasterSystem: {"Sega Master System"},
	core.PlatformGameGear:         {"Sega Game Gear"},
	core.PlatformSegaCD:           {"Sega CD", "Sega Mega-CD"},
	core.PlatformSega32X:          {"Sega 32X"},
	core.PlatformPS1:              {"Sony Playstation"},
	core.PlatformPS2:              {"Sony Playstation 2"},
	core.PlatformPS3:              {"Sony Playstation 3"},
	core.PlatformPSP:              {"Sony PSP"},
	core.PlatformXbox360:          {"Microsoft Xbox 360"},
	core.PlatformScummVM:          {"ScummVM", "MS-DOS", "Windows"},
}

const (
	dataDir         = "data"
	filesIndexFile  = "data/files-index.json"
	gamesIndexFile  = "data/games-index.json"
	imagesIndexFile = "data/images-index.json"
	timestampFile   = "data/.downloaded_at"
	metadataZipURL  = "https://gamesdb.launchbox-app.com/Metadata.zip"
	httpTimeout     = 10 * time.Minute
	maxAge          = 30 * 24 * time.Hour
)

var cache *launchBoxIndex

// --- Init ---

func handleInit() (any, *Error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, &Error{Code: "INIT_FAILED", Message: "create data dir: " + err.Error()}
	}

	downloadedAt := readTimestamp()
	hasIndex := fileExists(filesIndexFile) && fileExists(gamesIndexFile)

	if hasIndex && time.Since(downloadedAt) < maxAge {
		log.Printf("index is fresh (downloaded %s)", downloadedAt.Format(time.RFC3339))
		return map[string]any{"status": "ok", "cached": true}, nil
	}

	log.Println("downloading LaunchBox metadata...")
	if err := downloadAndBuildIndex(); err != nil {
		log.Printf("WARNING: download failed: %v", err)
		if hasIndex {
			log.Println("falling back to cached index")
			return map[string]any{"status": "ok", "cached": true}, nil
		}
		return nil, &Error{Code: "INIT_FAILED", Message: err.Error()}
	}

	cache = nil
	return map[string]any{"status": "ok"}, nil
}

func readTimestamp() time.Time {
	data, err := os.ReadFile(timestampFile)
	if err != nil {
		return time.Time{}
	}
	t, _ := time.Parse(time.RFC3339, strings.TrimSpace(string(data)))
	return t
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// --- Download & Index Build ---

func downloadAndBuildIndex() error {
	tmpFile, err := os.CreateTemp("", "launchbox-metadata-*.zip")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	log.Printf("downloading %s", metadataZipURL)
	client := &http.Client{Timeout: httpTimeout}
	resp, err := client.Get(metadataZipURL)
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

	var filesEntries []fileEntry
	var gamesEntries []gameEntry
	var imageEntries []gameImage

	for _, f := range zr.File {
		name := strings.ToLower(filepath.Base(f.Name))
		switch name {
		case "files.xml":
			log.Printf("parsing %s (%d bytes)", f.Name, f.UncompressedSize64)
			entries, err := parseFilesXML(f)
			if err != nil {
				return fmt.Errorf("parse Files.xml: %w", err)
			}
			filesEntries = append(filesEntries, entries...)
			log.Printf("parsed %d file entries", len(entries))

		case "mame.xml":
			log.Printf("parsing %s (%d bytes)", f.Name, f.UncompressedSize64)
			entries, err := parseMameXML(f)
			if err != nil {
				return fmt.Errorf("parse Mame.xml: %w", err)
			}
			filesEntries = append(filesEntries, entries...)
			log.Printf("parsed %d MAME entries", len(entries))

		case "metadata.xml":
			log.Printf("parsing %s (%d bytes)", f.Name, f.UncompressedSize64)
			games, images, err := parseMetadataXML(f)
			if err != nil {
				return fmt.Errorf("parse Metadata.xml: %w", err)
			}
			gamesEntries = games
			imageEntries = images
			log.Printf("parsed %d game entries, %d image entries", len(games), len(images))
		}
	}

	data, err := json.Marshal(filesEntries)
	if err != nil {
		return fmt.Errorf("marshal files index: %w", err)
	}
	if err := os.WriteFile(filesIndexFile, data, 0o644); err != nil {
		return fmt.Errorf("write files index: %w", err)
	}
	log.Printf("wrote files index: %d bytes", len(data))

	data, err = json.Marshal(gamesEntries)
	if err != nil {
		return fmt.Errorf("marshal games index: %w", err)
	}
	if err := os.WriteFile(gamesIndexFile, data, 0o644); err != nil {
		return fmt.Errorf("write games index: %w", err)
	}
	log.Printf("wrote games index: %d bytes", len(data))

	data, err = json.Marshal(imageEntries)
	if err != nil {
		return fmt.Errorf("marshal images index: %w", err)
	}
	if err := os.WriteFile(imagesIndexFile, data, 0o644); err != nil {
		return fmt.Errorf("write images index: %w", err)
	}
	log.Printf("wrote images index: %d bytes", len(data))

	if err := os.WriteFile(timestampFile, []byte(time.Now().UTC().Format(time.RFC3339)), 0o644); err != nil {
		return fmt.Errorf("write timestamp: %w", err)
	}

	return nil
}

// --- XML Parsing ---

func parseFilesXML(zf *zip.File) ([]fileEntry, error) {
	rc, err := zf.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return streamParseFiles(rc)
}

func streamParseFiles(r io.Reader) ([]fileEntry, error) {
	dec := xml.NewDecoder(r)
	var entries []fileEntry

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "File" {
			continue
		}
		e := parseFileElement(dec)
		if e.FileName != "" && e.GameName != "" && e.Platform != "" {
			entries = append(entries, e)
		}
	}
	return entries, nil
}

func parseFileElement(dec *xml.Decoder) fileEntry {
	var e fileEntry
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
			case "Platform":
				e.Platform = readText(dec, &depth)
			case "FileName":
				e.FileName = readText(dec, &depth)
			case "GameName":
				e.GameName = readText(dec, &depth)
			}
		case xml.EndElement:
			depth--
		}
	}
	return e
}

func parseMameXML(zf *zip.File) ([]fileEntry, error) {
	rc, err := zf.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return streamParseMame(rc)
}

func streamParseMame(r io.Reader) ([]fileEntry, error) {
	dec := xml.NewDecoder(r)
	var entries []fileEntry

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "MameFile" {
			continue
		}
		e := parseMameElement(dec)
		if e.FileName != "" && e.GameName != "" {
			entries = append(entries, e)
		}
	}
	return entries, nil
}

func parseMameElement(dec *xml.Decoder) fileEntry {
	var fileName, name string
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
			case "FileName":
				fileName = readText(dec, &depth)
			case "Name":
				name = readText(dec, &depth)
			}
		case xml.EndElement:
			depth--
		}
	}
	return fileEntry{Platform: "Arcade", FileName: fileName, GameName: name}
}

func parseMetadataXML(zf *zip.File) ([]gameEntry, []gameImage, error) {
	rc, err := zf.Open()
	if err != nil {
		return nil, nil, err
	}
	defer rc.Close()
	return streamParseMetadata(rc)
}

func streamParseMetadata(r io.Reader) ([]gameEntry, []gameImage, error) {
	dec := xml.NewDecoder(r)
	var games []gameEntry
	var images []gameImage

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, err
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch se.Name.Local {
		case "Game":
			e := parseGameElement(dec)
			if e.Name != "" && e.Platform != "" && e.DatabaseID != 0 {
				games = append(games, e)
			}
		case "GameImage":
			img := parseGameImageElement(dec)
			if img.DatabaseID != 0 && img.FileName != "" {
				images = append(images, img)
			}
		}
	}
	return games, images, nil
}

func parseGameImageElement(dec *xml.Decoder) gameImage {
	var img gameImage
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
			case "DatabaseID":
				text := readText(dec, &depth)
				fmt.Sscanf(text, "%d", &img.DatabaseID)
			case "FileName":
				img.FileName = readText(dec, &depth)
			case "Type":
				img.Type = readText(dec, &depth)
			}
		case xml.EndElement:
			depth--
		}
	}
	return img
}

func parseGameElement(dec *xml.Decoder) gameEntry {
	var e gameEntry
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
			case "DatabaseID":
				text := readText(dec, &depth)
				fmt.Sscanf(text, "%d", &e.DatabaseID)
			case "Name":
				e.Name = readText(dec, &depth)
			case "Platform":
				e.Platform = readText(dec, &depth)
			case "ReleaseYear":
				e.Year = readText(dec, &depth)
			case "Developer":
				e.Developer = readText(dec, &depth)
			case "Publisher":
				e.Publisher = readText(dec, &depth)
			case "Genres":
				e.Genres = readText(dec, &depth)
			case "Overview":
				e.Overview = readText(dec, &depth)
			case "MaxPlayers":
				e.MaxPlayers = readText(dec, &depth)
			case "VideoURL":
				e.VideoURL = readText(dec, &depth)
			case "CommunityRating":
				e.Rating = readText(dec, &depth)
			}
		case xml.EndElement:
			depth--
		}
	}
	return e
}

func readText(dec *xml.Decoder, depth *int) string {
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
			return strings.TrimSpace(sb.String())
		case xml.StartElement:
			*depth++
		}
	}
}

// --- Index Loading ---

func loadIndex() (*launchBoxIndex, error) {
	filesData, err := os.ReadFile(filesIndexFile)
	if err != nil {
		return nil, fmt.Errorf("read files index: %w", err)
	}
	gamesData, err := os.ReadFile(gamesIndexFile)
	if err != nil {
		return nil, fmt.Errorf("read games index: %w", err)
	}

	var fileEntries []fileEntry
	if err := json.Unmarshal(filesData, &fileEntries); err != nil {
		return nil, fmt.Errorf("unmarshal files index: %w", err)
	}
	var gameEntries []gameEntry
	if err := json.Unmarshal(gamesData, &gameEntries); err != nil {
		return nil, fmt.Errorf("unmarshal games index: %w", err)
	}

	var imgEntries []gameImage
	if imgData, err := os.ReadFile(imagesIndexFile); err == nil {
		json.Unmarshal(imgData, &imgEntries)
	}

	idx := &launchBoxIndex{
		files:      make(map[string]*fileEntry, len(fileEntries)),
		games:      make(map[string]*gameEntry, len(gameEntries)),
		normalized: make(map[string]*gameEntry, len(gameEntries)),
		byPlatform: make(map[string][]tokenedGame),
		images:     make(map[int][]gameImage),
	}

	for i := range fileEntries {
		e := &fileEntries[i]
		key := strings.ToLower(e.Platform) + "\t" + strings.ToLower(e.FileName)
		if _, exists := idx.files[key]; !exists {
			idx.files[key] = e
		}
	}
	for i := range gameEntries {
		e := &gameEntries[i]
		lp := strings.ToLower(e.Platform)

		key := lp + "\t" + strings.ToLower(e.Name)
		if _, exists := idx.games[key]; !exists {
			idx.games[key] = e
		}

		nkey := lp + "\t" + normalizeTitle(e.Name)
		if _, exists := idx.normalized[nkey]; !exists {
			idx.normalized[nkey] = e
		}

		tokens := tokenize(e.Name)
		if len(tokens) >= minMatchingTokens {
			idx.byPlatform[lp] = append(idx.byPlatform[lp], tokenedGame{tokens: tokens, game: e})
		}
	}
	for i := range imgEntries {
		img := imgEntries[i]
		idx.images[img.DatabaseID] = append(idx.images[img.DatabaseID], img)
	}

	log.Printf("loaded index: %d file mappings, %d game entries, %d normalized, %d platforms with token index, %d images",
		len(idx.files), len(idx.games), len(idx.normalized), len(idx.byPlatform), len(imgEntries))
	return idx, nil
}

func getOrLoadIndex() (*launchBoxIndex, error) {
	if cache != nil {
		return cache, nil
	}
	idx, err := loadIndex()
	if err != nil {
		return nil, err
	}
	cache = idx
	return idx, nil
}

// --- Lookup ---

func handleLookup(params lookupParams) (any, *Error) {
	idx, err := getOrLoadIndex()
	if err != nil {
		return nil, &Error{Code: "INDEX_NOT_READY", Message: err.Error()}
	}

	var results []lookupResult
	for _, q := range params.Games {
		if q.LookupIntent == "manual_search" {
			matches := matchGamesForManualSearch(idx, q)
			results = append(results, matches...)
			if len(matches) > 0 {
				continue
			}
			log.Printf(
				"launchbox manual lookup miss: index=%d title=%q platform=%q root_path=%q",
				q.Index,
				q.Title,
				q.Platform,
				q.RootPath,
			)
			continue
		}
		if r := matchGame(idx, q); r != nil {
			results = append(results, *r)
			continue
		}
		log.Printf(
			"launchbox lookup miss: index=%d title=%q platform=%q root_path=%q",
			q.Index,
			q.Title,
			q.Platform,
			q.RootPath,
		)
	}

	return map[string]any{"results": results}, nil
}

func matchGamesForManualSearch(idx *launchBoxIndex, q gameQuery) []lookupResult {
	lbPlatforms := lookupPlatformsForQuery(q.Platform)
	if len(lbPlatforms) == 0 {
		lbPlatforms = fallbackPlatforms(q)
	}
	if len(lbPlatforms) == 0 {
		lbPlatforms = []string{q.Platform}
	}

	type candidate struct {
		entry *gameEntry
		score float64
	}
	candidates := map[int]candidate{}
	add := func(ge *gameEntry, score float64) {
		if ge == nil {
			return
		}
		current, ok := candidates[ge.DatabaseID]
		if !ok || score > current.score {
			candidates[ge.DatabaseID] = candidate{entry: ge, score: score}
		}
	}

	filename := strings.TrimSuffix(q.Title, filepath.Ext(q.Title))
	for _, lbp := range lbPlatforms {
		if fe := idx.lookupFile(lbp, filename); fe != nil {
			add(idx.lookupGame(fe.Platform, fe.GameName), 1.0)
		}
	}

	for _, variant := range titlematch.LookupTitleVariants(q.Title) {
		for _, lbp := range lbPlatforms {
			add(idx.lookupGame(lbp, variant), 0.99)
			add(idx.lookupNormalized(lbp, variant), 0.98)
		}
		normTitle := normalizeTitle(variant)
		for _, titleVariant := range titleVariations(normTitle) {
			for _, lbp := range lbPlatforms {
				add(idx.normalized[strings.ToLower(lbp)+"\t"+titleVariant], 0.94)
			}
		}
		queryTokens := tokenize(variant)
		if len(queryTokens) >= minMatchingTokens {
			for _, lbp := range lbPlatforms {
				for _, match := range tokenMatches(queryTokens, idx.byPlatform[strings.ToLower(lbp)]) {
					add(match.game, match.score)
				}
			}
		}
	}

	ranked := make([]candidate, 0, len(candidates))
	for _, candidate := range candidates {
		ranked = append(ranked, candidate)
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		if ranked[i].entry.Name != ranked[j].entry.Name {
			return ranked[i].entry.Name < ranked[j].entry.Name
		}
		return ranked[i].entry.DatabaseID < ranked[j].entry.DatabaseID
	})
	if len(ranked) > maxManualResults {
		ranked = ranked[:maxManualResults]
	}

	results := make([]lookupResult, 0, len(ranked))
	for _, item := range ranked {
		if result := buildResult(q.Index, item.entry, idx); result != nil {
			results = append(results, *result)
		}
	}
	return results
}

func matchGame(idx *launchBoxIndex, q gameQuery) *lookupResult {
	lbPlatforms := lookupPlatformsForQuery(q.Platform)
	if len(lbPlatforms) == 0 {
		lbPlatforms = fallbackPlatforms(q)
	}
	if len(lbPlatforms) == 0 {
		lbPlatforms = []string{q.Platform}
	}

	filename := strings.TrimSuffix(q.Title, filepath.Ext(q.Title))

	// Strategy 1: file-based matching (ROM filename → game name → metadata).
	for _, lbp := range lbPlatforms {
		fe := idx.lookupFile(lbp, filename)
		if fe == nil {
			continue
		}
		ge := idx.lookupGame(fe.Platform, fe.GameName)
		if ge != nil {
			return buildResult(q.Index, ge, idx)
		}
		return &lookupResult{
			Index:      q.Index,
			Title:      fe.GameName,
			ExternalID: "lb-file:" + fe.GameName,
		}
	}

	// Strategy 2: exact title matching.
	for _, lbp := range lbPlatforms {
		ge := idx.lookupGame(lbp, q.Title)
		if ge != nil {
			return buildResult(q.Index, ge, idx)
		}
	}

	// Strategy 3a: normalized title matching (strips versions, punctuation, collapses whitespace).
	normTitle := normalizeTitle(q.Title)
	for _, lbp := range lbPlatforms {
		lp := strings.ToLower(lbp)
		if ge := idx.normalized[lp+"\t"+normTitle]; ge != nil {
			return buildResult(q.Index, ge, idx)
		}
	}

	// Strategy 3b: title variations (roman ↔ arabic numerals, drop trailing "1").
	for _, variant := range titleVariations(normTitle) {
		for _, lbp := range lbPlatforms {
			lp := strings.ToLower(lbp)
			if ge := idx.normalized[lp+"\t"+variant]; ge != nil {
				return buildResult(q.Index, ge, idx)
			}
		}
	}

	// Strategy 4: token overlap — Jaccard similarity with threshold.
	queryTokens := tokenize(q.Title)
	if len(queryTokens) >= minMatchingTokens {
		for _, lbp := range lbPlatforms {
			candidates := idx.byPlatform[strings.ToLower(lbp)]
			if ge := bestTokenMatch(queryTokens, candidates); ge != nil {
				return buildResult(q.Index, ge, idx)
			}
		}
	}

	return nil
}

func lookupPlatformsForQuery(platform string) []string {
	canonical := core.NormalizePlatformAlias(platform)
	if canonical != core.PlatformUnknown {
		if aliases := launchBoxPlatformAliases[canonical]; len(aliases) > 0 {
			return aliases
		}
		return nil
	}
	platform = strings.TrimSpace(platform)
	if platform == "" || strings.EqualFold(platform, string(core.PlatformUnknown)) {
		return nil
	}
	return []string{platform}
}

func fallbackPlatforms(q gameQuery) []string {
	if q.Platform != "" && q.Platform != "unknown" {
		return nil
	}
	root := strings.ToLower(filepath.ToSlash(q.RootPath))
	if q.GroupKind == "packed" || strings.Contains(root, "installers") {
		return []string{"Windows"}
	}
	return nil
}

func buildResult(index int, ge *gameEntry, idx *launchBoxIndex) *lookupResult {
	r := &lookupResult{
		Index:       index,
		Title:       ge.Name,
		ExternalID:  fmt.Sprintf("%d", ge.DatabaseID),
		URL:         fmt.Sprintf("https://gamesdb.launchbox-app.com/games/details/%d", ge.DatabaseID),
		Description: ge.Overview,
		Developer:   ge.Developer,
		Publisher:   ge.Publisher,
	}
	if platform := core.NormalizePlatformAlias(ge.Platform); platform != core.PlatformUnknown {
		r.Platform = string(platform)
	}

	if ge.Year != "" {
		r.ReleaseDate = ge.Year
	}
	if ge.Genres != "" {
		for _, g := range strings.Split(ge.Genres, ";") {
			g = strings.TrimSpace(g)
			if g != "" {
				r.Genres = append(r.Genres, g)
			}
		}
	}
	if ge.MaxPlayers != "" {
		var mp int
		fmt.Sscanf(ge.MaxPlayers, "%d", &mp)
		r.MaxPlayers = mp
	}
	if ge.Rating != "" {
		var rating float64
		fmt.Sscanf(ge.Rating, "%f", &rating)
		if rating > 0 {
			r.Rating = rating * 20
		}
	}
	if ge.VideoURL != "" {
		r.Media = append(r.Media, mediaItemIPC{Type: "video", URL: ge.VideoURL})
	}

	const lbImageBase = "https://images.launchbox-app.com/"
	for _, img := range idx.images[ge.DatabaseID] {
		imgURL := lbImageBase + img.FileName
		switch img.Type {
		case "Box - Front", "Box - Front - Reconstructed":
			r.Media = append(r.Media, mediaItemIPC{Type: "cover", URL: imgURL})
		case "Box - Back", "Box - Back - Reconstructed":
			r.Media = append(r.Media, mediaItemIPC{Type: "box_back", URL: imgURL})
		case "Screenshot - Gameplay", "Screenshot - Game Title", "Screenshot - Game Select", "Screenshot - Game Over":
			r.Media = append(r.Media, mediaItemIPC{Type: "screenshot", URL: imgURL})
		case "Clear Logo":
			r.Media = append(r.Media, mediaItemIPC{Type: "logo", URL: imgURL})
		case "Banner":
			r.Media = append(r.Media, mediaItemIPC{Type: "banner", URL: imgURL})
		case "Fanart - Background":
			r.Media = append(r.Media, mediaItemIPC{Type: "background", URL: imgURL})
		case "Arcade - Cabinet", "Arcade - Cabinet Left", "Arcade - Cabinet Right":
			r.Media = append(r.Media, mediaItemIPC{Type: "cabinet", URL: imgURL})
		case "Arcade - Marquee":
			r.Media = append(r.Media, mediaItemIPC{Type: "marquee", URL: imgURL})
		}
	}

	return r
}

// --- Main ---

func main() {
	log.SetOutput(os.Stderr)
	log.Println("LaunchBox metadata plugin started")

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
				"plugin_id":      "metadata-launchbox",
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
