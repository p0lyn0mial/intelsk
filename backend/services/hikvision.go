package services

import (
	"crypto/md5"
	"crypto/tls"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/intelsk/backend/models"
)

// HikvisionClient communicates with a Hikvision device (camera or NVR) via ISAPI over HTTPS.
type HikvisionClient struct {
	ip       string
	username string
	password string
	client   *http.Client
}

func NewHikvisionClient(ip, username, password string) *HikvisionClient {
	return &HikvisionClient{
		ip:       ip,
		username: username,
		password: password,
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		},
	}
}

// Recording represents a single recording found on the NVR.
type Recording struct {
	SourceID    string
	TrackID     int
	StartTime   time.Time
	EndTime     time.Time
	PlaybackURI string
}

// Snapshot fetches a JPEG snapshot from the given channel.
func (c *HikvisionClient) Snapshot(channel int) ([]byte, error) {
	url := fmt.Sprintf("https://%s/ISAPI/Streaming/channels/%d01/picture", c.ip, channel)
	resp, err := c.doDigest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("snapshot request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("snapshot returned %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// Ping checks if the device is reachable.
// Any HTTP response (even 401/403) means the NVR is online — only network errors are failures.
func (c *HikvisionClient) Ping() error {
	url := fmt.Sprintf("https://%s/ISAPI/System/status", c.ip)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// DeviceInfo holds basic information about a Hikvision device.
type DeviceInfo struct {
	DeviceName   string `json:"device_name"`
	Model        string `json:"model"`
	SerialNumber string `json:"serial_number"`
	FirmwareVer  string `json:"firmware_version"`
	Channels     int    `json:"channels"`
}

// XML types for ISAPI device info
type deviceInfoXML struct {
	XMLName          xml.Name `xml:"DeviceInfo"`
	DeviceName       string   `xml:"deviceName"`
	Model            string   `xml:"model"`
	SerialNumber     string   `xml:"serialNumber"`
	FirmwareVersion  string   `xml:"firmwareVersion"`
}

type videoInputChannelListXML struct {
	XMLName  xml.Name                `xml:"VideoInputChannelList"`
	Channels []videoInputChannelXML  `xml:"VideoInputChannel"`
}

type videoInputChannelXML struct {
	ID   int    `xml:"id"`
	Name string `xml:"name"`
}

// GetDeviceInfo fetches authenticated device info and channel count from the NVR.
// Unlike Ping, this verifies that credentials are correct.
func (c *HikvisionClient) GetDeviceInfo() (*DeviceInfo, error) {
	url := fmt.Sprintf("https://%s/ISAPI/System/deviceInfo", c.ip)
	resp, err := c.doDigest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("device info request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("authentication failed: check username and password")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("device info returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading device info: %w", err)
	}

	var xmlInfo deviceInfoXML
	if err := xml.Unmarshal(body, &xmlInfo); err != nil {
		return nil, fmt.Errorf("parsing device info: %w", err)
	}

	info := &DeviceInfo{
		DeviceName:   xmlInfo.DeviceName,
		Model:        xmlInfo.Model,
		SerialNumber: xmlInfo.SerialNumber,
		FirmwareVer:  xmlInfo.FirmwareVersion,
	}

	// Try to get channel count
	chURL := fmt.Sprintf("https://%s/ISAPI/System/Video/inputs/channels", c.ip)
	chResp, err := c.doDigest("GET", chURL, nil)
	if err == nil {
		defer chResp.Body.Close()
		if chResp.StatusCode == http.StatusOK {
			chBody, _ := io.ReadAll(chResp.Body)
			var chList videoInputChannelListXML
			if xml.Unmarshal(chBody, &chList) == nil {
				info.Channels = len(chList.Channels)
			}
		}
	}

	return info, nil
}

// SearchRecordings searches for recordings on the NVR for a given channel and time range.
func (c *HikvisionClient) SearchRecordings(channel int, start, end time.Time) ([]Recording, error) {
	trackID := channel*100 + 1 // e.g., channel 1 → trackID 101
	searchXML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<CMSearchDescription>
  <searchID>%s</searchID>
  <trackIDList>
    <trackID>%d</trackID>
  </trackIDList>
  <timeSpanList>
    <timeSpan>
      <startTime>%s</startTime>
      <endTime>%s</endTime>
    </timeSpan>
  </timeSpanList>
  <maxResults>500</maxResults>
  <searchResultPostion>0</searchResultPostion>
  <metadataList>
    <metadataDescriptor>//recordType.meta.std-cgi.com</metadataDescriptor>
  </metadataList>
</CMSearchDescription>`,
		generateSearchID(),
		trackID,
		start.Format("2006-01-02T15:04:05Z"),
		end.Format("2006-01-02T15:04:05Z"),
	)

	url := fmt.Sprintf("https://%s/ISAPI/ContentMgmt/search", c.ip)
	resp, err := c.doDigest("POST", url, strings.NewReader(searchXML))
	if err != nil {
		return nil, fmt.Errorf("search request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("search returned %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading search response: %w", err)
	}

	return parseSearchResults(body)
}

// DownloadClip downloads a recording from the NVR to the given output path.
// Uses POST /ISAPI/ContentMgmt/download with the playbackURI from search results.
func (c *HikvisionClient) DownloadClip(playbackURI, outputPath string) error {
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	// XML-encode & in the playbackURI (required by ISAPI)
	safeURI := strings.ReplaceAll(playbackURI, "&", "&amp;")
	downloadXML := fmt.Sprintf(`<downloadRequest><playbackURI>%s</playbackURI></downloadRequest>`, safeURI)

	downloadURL := fmt.Sprintf("https://%s/ISAPI/ContentMgmt/download", c.ip)

	// Use a longer timeout for downloads
	oldTimeout := c.client.Timeout
	c.client.Timeout = 30 * time.Minute
	defer func() { c.client.Timeout = oldTimeout }()

	resp, err := c.doDigest("GET", downloadURL, strings.NewReader(downloadXML))
	if err != nil {
		return fmt.Errorf("download request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("download returned %d: %s", resp.StatusCode, string(body))
	}

	out, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("creating output file: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		os.Remove(outputPath)
		return fmt.Errorf("saving download: %w", err)
	}
	return nil
}

// doDigest performs an HTTP request with Digest Authentication.
func (c *HikvisionClient) doDigest(method, url string, body io.Reader) (*http.Response, error) {
	// First request without auth to get the WWW-Authenticate challenge
	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = io.ReadAll(body)
		if err != nil {
			return nil, err
		}
	}

	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusUnauthorized {
		// No auth needed or different error
		if bodyBytes != nil {
			resp.Body.Close()
			req2, _ := http.NewRequest(method, url, strings.NewReader(string(bodyBytes)))
			return c.client.Do(req2)
		}
		return resp, nil
	}
	resp.Body.Close()

	// Parse WWW-Authenticate header
	authHeader := resp.Header.Get("WWW-Authenticate")
	if authHeader == "" {
		return nil, fmt.Errorf("no WWW-Authenticate header in 401 response")
	}

	params := parseDigestChallenge(authHeader)
	realm := params["realm"]
	nonce := params["nonce"]
	qop := params["qop"]

	// Compute digest response
	ha1 := md5Hex(fmt.Sprintf("%s:%s:%s", c.username, realm, c.password))
	ha2 := md5Hex(fmt.Sprintf("%s:%s", method, urlPath(url)))

	nc := "00000001"
	cnonce := fmt.Sprintf("%08x", rand.Int31())

	var response string
	if strings.Contains(qop, "auth") {
		response = md5Hex(fmt.Sprintf("%s:%s:%s:%s:%s:%s", ha1, nonce, nc, cnonce, "auth", ha2))
	} else {
		response = md5Hex(fmt.Sprintf("%s:%s:%s", ha1, nonce, ha2))
	}

	authValue := fmt.Sprintf(
		`Digest username="%s", realm="%s", nonce="%s", uri="%s", response="%s"`,
		c.username, realm, nonce, urlPath(url), response,
	)
	if strings.Contains(qop, "auth") {
		authValue += fmt.Sprintf(`, qop=auth, nc=%s, cnonce="%s"`, nc, cnonce)
	}
	if opaque, ok := params["opaque"]; ok {
		authValue += fmt.Sprintf(`, opaque="%s"`, opaque)
	}

	// Retry with auth
	var reqBody io.Reader
	if bodyBytes != nil {
		reqBody = strings.NewReader(string(bodyBytes))
	}
	req2, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, err
	}
	req2.Header.Set("Authorization", authValue)
	if bodyBytes != nil {
		req2.Header.Set("Content-Type", "application/xml")
	}
	return c.client.Do(req2)
}

func parseDigestChallenge(header string) map[string]string {
	params := make(map[string]string)
	header = strings.TrimPrefix(header, "Digest ")
	parts := strings.Split(header, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		eq := strings.IndexByte(part, '=')
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(part[:eq])
		val := strings.TrimSpace(part[eq+1:])
		val = strings.Trim(val, `"`)
		params[key] = val
	}
	return params
}

func md5Hex(s string) string {
	h := md5.Sum([]byte(s))
	return hex.EncodeToString(h[:])
}

func urlPath(rawURL string) string {
	// Extract path from URL
	idx := strings.Index(rawURL, "://")
	if idx >= 0 {
		rest := rawURL[idx+3:]
		slash := strings.IndexByte(rest, '/')
		if slash >= 0 {
			return rest[slash:]
		}
	}
	return rawURL
}

func generateSearchID() string {
	return fmt.Sprintf("search-%d", time.Now().UnixNano())
}

// XML response parsing for search results

type cmSearchResult struct {
	XMLName      xml.Name       `xml:"CMSearchResult"`
	ResponseURL  string         `xml:"responseStatusStrg"`
	NumOfMatches int            `xml:"numOfMatches"`
	MatchList    matchListXML   `xml:"matchList"`
}

type matchListXML struct {
	Matches []searchMatchXML `xml:"searchMatchItem"`
}

type searchMatchXML struct {
	SourceID    string      `xml:"sourceID"`
	TrackID     int         `xml:"trackID"`
	TimeSpan    timeSpanXML `xml:"timeSpan"`
	MediaSegmentDescriptor struct {
		PlaybackURI string `xml:"playbackURI"`
	} `xml:"mediaSegmentDescriptor"`
}

type timeSpanXML struct {
	StartTime string `xml:"startTime"`
	EndTime   string `xml:"endTime"`
}

func parseSearchResults(data []byte) ([]Recording, error) {
	var result cmSearchResult
	if err := xml.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parsing search XML: %w", err)
	}

	var recordings []Recording
	for _, m := range result.MatchList.Matches {
		start, _ := time.Parse("2006-01-02T15:04:05Z", m.TimeSpan.StartTime)
		end, _ := time.Parse("2006-01-02T15:04:05Z", m.TimeSpan.EndTime)
		recordings = append(recordings, Recording{
			SourceID:    m.SourceID,
			TrackID:     m.TrackID,
			StartTime:   start,
			EndTime:     end,
			PlaybackURI: m.MediaSegmentDescriptor.PlaybackURI,
		})
	}
	return recordings, nil
}

// RTSPUrl builds the RTSP URL for a Hikvision camera channel via the NVR.
// streamType: 1 = main stream (high res), 2 = sub stream (low res).
func RTSPUrl(ip string, rtspPort int, username, password string, channel, streamType int) string {
	return fmt.Sprintf("rtsp://%s:%s@%s:%d/Streaming/Channels/%d0%d",
		username, password, ip, rtspPort, channel, streamType)
}

// NVRChannel extracts the channel number from a hikvision camera config.
func NVRChannel(cam *models.CameraInfo) int {
	if ch, ok := cam.Config["nvr_channel"].(float64); ok && ch >= 1 {
		return int(ch)
	}
	return 1
}
