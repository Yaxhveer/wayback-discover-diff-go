package job

import (
	"compress/flate"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"wayback-discover-diff-go/internal/simhash"
	"wayback-discover-diff-go/internal/utils"

	"github.com/redis/go-redis/v9"
)

const MAP_CAPTURE_DOWNLOAD = 1000000
const MAX_DOWNLOAD_ERRORS = 10
const MAX_RETRIES = 2
const CONCURRENCY_LIMIT = 20

var mu sync.Mutex
var simhashMap map[string]string = make(map[string]string)

// Job manages a queue of jobs.
// instead of passing everything we would pass config as parameter which would be further used
type Job struct {
	ID         string
	URL        string
	Year       string
	State      string
	Info       string
	startTime  time.Time
	Duration   time.Duration
	HTTPClient *http.Client
	Headers    map[string]string
}

// NewJobQueue initializes the job queue with an HTTP client.
func NewJobQueue() *Job {
	headers := map[string]string{
		"User-Agent":      "wayback-discover-diff",
		"Accept-Encoding": "gzip,deflate",
		"Connection":      "keep-alive",
	}

	// load from config
	var cdxAuthToken string = "xxxx-yyy-zzz-www-xxxxx"

	if cdxAuthToken != "" {
		headers["cookie"] = "cdx_auth_token=" + cdxAuthToken
	}

	client := &http.Client{
		Timeout: 35 * time.Second, // Slightly increased to handle slow responses
		Transport: &http.Transport{
			MaxIdleConns:        500,              // Increase idle connections for reuse
			MaxIdleConnsPerHost: 250,              // Limit per host to prevent overloading
			MaxConnsPerHost:     300,              // Control parallel connections per host
			IdleConnTimeout:     60 * time.Second, // Keep idle conns open for reuse
		},
	}
	return &Job{
		HTTPClient: client,
		Headers:    headers,
	}
}

// RunJob executes a new job and returns the job_id
func (j *Job) RunJob(redisClient redis.Client, url, year string) string {
	j.startTime = time.Now()
	jobID := fmt.Sprintf("%x", sha256.Sum256([]byte(url+year+time.Now().String())))

	j.ID = jobID
	j.URL = url
	j.Year = year
	j.State = "PENDING"
	j.Info = fmt.Sprintf("Fetching %s captures for year %s", url, year)

	go func() {
		// Fetch CDX captures
		captures, err := j.FetchCDX(url, year)
		if err != nil {
			j.State = "ERROR"
			j.Info = fmt.Sprintf("error while fetching cdx for url %s and year %s, %s\n", url, year, err.Error())
			fmt.Println(j.Info)
			return
		}

		totalCaptures := len(captures)
		finalResult := map[string]string{}
		// Process each capture concurrently
		var wg sync.WaitGroup
		workerCh := make(chan struct{}, CONCURRENCY_LIMIT)
		var i int64
		for _, capture := range captures {
			workerCh <- struct{}{}
			wg.Add(1)
			go func(capture string) {
				defer func() {
					<-workerCh
					wg.Done()
				}()
				timestamp, simhash := j.GetCalculation(capture)
				if timestamp != "" && simhash != "" {
					finalResult[timestamp] = simhash

					if i%10 == 0 {
						j.State = "PENDING"
						j.Info = fmt.Sprintf("Processed %d out of %d captures.\n", i, totalCaptures)
					}
					atomic.AddInt64(&i, 1)
				}
			}(capture)
		}
		wg.Wait()

		j.State = "COMPLETE"
		j.Info = fmt.Sprintf("Processed %d captures.\n", totalCaptures)

		if len(finalResult) != 0 {
			urlKey := utils.Surt(url)
			err := redisClient.HSet(context.Background(), urlKey, finalResult).Err()
			if err != nil {
				j.Info = fmt.Sprintf("cannot write simhashes to Redis for URL %s, %s", url, err.Error())
				fmt.Println(j.Info)
				return
			}

			// load from config
			var expire time.Duration = 86400
			err = redisClient.Expire(context.Background(), urlKey, expire*time.Second).Err()
			if err != nil {
				j.Info = fmt.Sprintf("cannot write simhashes to Redis for URL %s, %s", url, err.Error())
				fmt.Println(j.Info)
				return
			}
		}

		duration := time.Now().Sub(j.startTime)
		j.Duration = duration
		fmt.Printf("Simhash calculation finished in %.2fsec.\n", duration.Seconds())
		return
	}()

	return jobID
}

// FetchCDX fetches captures for a given URL and year.
func (j *Job) FetchCDX(targetURL, year string) ([]string, error) {
	fmt.Printf("fetching CDX of url %s for year %s\n", targetURL, year)

	params := url.Values{}
	params.Set("url", targetURL)
	params.Set("from", year)
	params.Set("to", year)
	params.Set("statuscode", "200")
	params.Set("fl", "timestamp,digest")
	params.Set("collapse", "timestamp:9")

	// load from config
	snapShotsNumber := -1
	if snapShotsNumber != -1 {
		params.Set("limit", strconv.Itoa(snapShotsNumber))
	}

	apiURL := "https://web.archive.org/web/timemap?" + params.Encode()

	fmt.Printf("api: %s\n", apiURL)
	fmt.Printf("Generating the get fetch req Request, %f\n", time.Now().Sub(j.startTime).Seconds())

	req, err := j.generateGetRequest(apiURL)
	if err != nil {
		return nil, err
	}

	fmt.Printf("Making the get fetch req Request, %f\n", time.Now().Sub(j.startTime).Seconds())

	resp, err := j.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("No captures of %s for year %s, %s", targetURL, year, err.Error())
	}

	fmt.Printf("Completed the get Request of fetch, %f\n", time.Now().Sub(j.startTime).Seconds())
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Failed request to %s, status: %d, response: %s", apiURL, resp.StatusCode, string(errBody))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	captures := strings.Split(strings.TrimSpace(string(body)), "\n")
	if len(captures) == 0 || (len(captures) == 1 && captures[0] == "") {
		return nil, fmt.Errorf("No captures of %s for year", apiURL, year)
	}

	fmt.Printf("captured %d CDX of url %s for year %s\n", len(captures), targetURL, year)
	return captures, nil
}

// GetCalculation processes a single capture and returns (timestamp, simhash).
func (j *Job) GetCalculation(capture string) (string, string) {
	parts := strings.Split(capture, " ")
	if len(parts) < 2 {
		return "", ""
	}
	timestamp, digest := parts[0], parts[1]

	// Check if digest is already processed
	mu.Lock()
	if simhash, exists := simhashMap[digest]; exists {
		fmt.Printf("already seen %s\n", digest)
		return timestamp, simhash
	}
	mu.Unlock()

	// Simulate download (placeholder for actual implementation)
	respData := j.DownloadCapture(timestamp)
	if len(respData) == 0 {
		return "", ""
	}

	// Extract HTML features
	features := extractHTMLFeatures(respData)
	if len(features) == 0 {
		return "", ""
	}

	// Compute SimHash
	fmt.Printf("calculating simhash\n")

	// get from config
	simhashSize := 256
	encodedSimhash := simhash.GetSimhash(features, simhashSize)

	// Store result
	mu.Lock()
	simhashMap[digest] = encodedSimhash
	mu.Unlock()
	return timestamp, encodedSimhash
}

func (j *Job) DownloadCapture(timestamp string) string {
	fmt.Printf("fetching capture %s %s\n", timestamp, j.URL)
	apiURL := fmt.Sprintf("https://web.archive.org/web/%sid_/%s", timestamp, j.URL)

	var resp *http.Response

	for i := 0; i < MAX_RETRIES; i++ {
		time.Sleep(exponentialBackoff(i))
		req, err := j.generateGetRequest(apiURL)
		if err != nil {
			fmt.Printf("cannot fetch capture %s %s, %s\n", timestamp, j.URL, err.Error())
			continue
		}

		resp, err = j.HTTPClient.Do(req)
		if err != nil {
			fmt.Printf("cannot fetch capture %s %s, %s\n", timestamp, j.URL, err.Error())
			continue
		}

		break
	}

	if resp == nil {
		return ""
	}
	defer resp.Body.Close()

	// Handle gzip and deflate decompression
	var reader io.Reader = io.LimitReader(resp.Body, int64(MAP_CAPTURE_DOWNLOAD))
	if strings.Contains(resp.Header.Get("Content-Encoding"), "gzip") {
		gzReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			fmt.Printf("cannot decompress gzip response %s %s, %s\n", timestamp, j.URL, err.Error())
			return ""
		}
		defer gzReader.Close()
		reader = gzReader
	} else if strings.Contains(resp.Header.Get("Content-Encoding"), "deflate") {
		deflateReader := flate.NewReader(resp.Body)
		defer deflateReader.Close()
		reader = deflateReader
	}

	// Read decompressed response body
	data, err := io.ReadAll(reader)
	if err != nil {
		fmt.Printf("cannot read response body %s %s, %s\n", timestamp, j.URL, err.Error())
		return ""
	}

	// Check if it's text-based content
	cType := strings.ToLower(resp.Header.Get("Content-Type"))
	if strings.Contains(cType, "text") || strings.Contains(cType, "html") {
		return string(data)
	}

	return ""
}

func exponentialBackoff(retry int) time.Duration {
	base := 1000 * time.Microsecond
	// Exponential backoff: base * 2^retry, plus some jitter
	jitter := time.Duration(rand.Intn(2000)) * time.Microsecond
	return base*time.Duration(math.Pow(2, float64(retry))) + jitter
}

func (j *Job) generateGetRequest(apiURL string) (*http.Request, error) {
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}

	for k, v := range j.Headers {
		req.Header.Set(k, v)
	}
	return req, nil
}
