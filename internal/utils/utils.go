package utils

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/net/publicsuffix"
)

type CaptureResult struct {
	Timestamp string
	Simhash   string
}

// YearSimhash retrieves stored simhash data from Redis for a given URL and year.
func YearSimhash(redisClient *redis.Client, url, year string, page, snapshotsPerPage int) ([]CaptureResult, error) {
	if url == "" || year == "" {
		return nil, errors.New("invalid URL or timestamp")
	}

	key := Surt(url)
	ctx := context.Background()

	timestamps, err := redisClient.HKeys(ctx, key).Result()
	if err != nil || len(timestamps) == 0 {
		return nil, fmt.Errorf("NO_CAPTURES")
	}
	slices.Sort(timestamps)
	var timeStampsToFetch []string
	for _, ts := range timestamps {
		if ts == year {
			return nil, fmt.Errorf("NO_CAPTURES")
		}
		if strings.HasPrefix(ts, year) {
			timeStampsToFetch = append(timeStampsToFetch, ts)
		}
	}

	if len(timeStampsToFetch) == 0 {
		return nil, fmt.Errorf("NO_CAPTURES")
	}

	return handleResults(redisClient, timeStampsToFetch, key, snapshotsPerPage, page), nil
}

func handleResults(redisClient *redis.Client, timestamps []string, key string, snapshotsPerPage, page int) []CaptureResult {
	if page != -1 {
		totalPages := int(math.Ceil(float64(len(timestamps)) / float64(snapshotsPerPage)))
		if totalPages > 0 {
			page = min(page, totalPages)
			start := (page - 1) * snapshotsPerPage
			end := min(page*snapshotsPerPage, len(timestamps))
			timestamps = timestamps[start:end]
		}
	}
	// if page != -1 {
	// 	start := (page - 1) * snapshotsPerPage
	// 	if start >= len(timestamps) {
	// 		return nil
	// 	}
	// 	end := min(page*snapshotsPerPage, len(timestamps))
	// 	timestamps = timestamps[start:end]
	// }

	results, err := redisClient.HMGet(context.Background(), key, timestamps...).Result()
	if err != nil {
		log.Printf("cannot fetch results for %s page %d: %v", key, page, err)
		return nil
	}

	captureResults := make([]CaptureResult, 0, len(results))
	for i, simhash := range results {
		if simhashStr, ok := simhash.(string); ok {
			captureResults = append(captureResults, CaptureResult{Timestamp: timestamps[i], Simhash: simhashStr})
		}
	}
	return captureResults
}

// TimestampSimHash retrieves stored simhash data from Redis for a given URL and timestamp.
func TimestampSimHash(redisClient *redis.Client, url, timestamp string) (map[string]string, error) {
	if url == "" || timestamp == "" || !validateTimestamp(timestamp) {
		return nil, errors.New("invalid URL or timestamp")
	}
	key := Surt(url)

	result, err := redisClient.HGet(context.Background(), key, timestamp).Result()
	if err != nil {
		return nil, fmt.Errorf("error loading simhash data for url %s timestamp %s (%s)", url, timestamp, err)
	} else if len(result) != 0 {
		return map[string]string{"simhash": result}, nil
	}

	result, err = redisClient.HGet(context.Background(), key, timestamp[:4]).Result()
	if err != nil {
		return nil, fmt.Errorf("error loading simhash data for url %s timestamp %s (%s)", url, timestamp, err)
	} else if len(result) != 0 {
		return map[string]string{"status": "error", "message": "NO_CAPTURES"}, nil
	}

	return map[string]string{"status": "error", "message": "CAPTURE_NOT_FOUND"}, nil
}

// Surt converts a URL into a SURT (Sort-friendly URI Reordering Transform)
func Surt(url string) string {
	domainParts := strings.Split(url, ".")
	sort.Strings(domainParts)
	return strings.Join(domainParts, ",")
}

// URLIsValid validates the URL using regex.
var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9_.+-]+@[a-zA-Z0-9-]+\.[a-zA-Z0-9-.]+$`)

func URLIsValid(url string) bool {
	if url == "" || emailRegex.MatchString(url) {
		return false
	}
	suffix, _ := publicsuffix.PublicSuffix(url)
	return suffix != ""
}

// CompressCaptures compresses capture timestamps and returns structured data.
func CompressCaptures(captures []CaptureResult) ([][]interface{}, []string) {
	hashDict := make(map[string]int)
	grouped := make(map[int]map[int]map[int][][]interface{})

	for _, capture := range captures {
		ts, simhash := capture.Timestamp, capture.Simhash
		year, month, day := atoi(ts[:4]), atoi(ts[4:6]), atoi(ts[6:8])
		hms := ts[8:]

		if _, exists := hashDict[simhash]; !exists {
			hashDict[simhash] = len(hashDict)
		}
		hashID := hashDict[simhash]

		if _, ok := grouped[year]; !ok {
			grouped[year] = make(map[int]map[int][][]interface{})
		}
		if _, ok := grouped[year][month]; !ok {
			grouped[year][month] = make(map[int][][]interface{})
		}
		grouped[year][month][day] = append(grouped[year][month][day], []interface{}{hms, hashID})
	}

	newCaptures := make([][]interface{}, 0)
	for y, yc := range grouped {
		yearEntry := []interface{}{y}
		for m, mc := range yc {
			monthEntry := []interface{}{m}
			for d, dc := range mc {
				monthEntry = append(monthEntry, []interface{}{d, dc})
			}
			yearEntry = append(yearEntry, monthEntry)
		}
		newCaptures = append(newCaptures, yearEntry)
	}

	sortedHashes := make([]string, len(hashDict))
	for hash, id := range hashDict {
		sortedHashes[id] = hash
	}

	return newCaptures, sortedHashes
}

// Atoi safely converts string to int
func atoi(s string) int {
	val, _ := strconv.Atoi(s)
	return val
}

func validateTimestamp(ts string) bool {
	_, err := time.Parse("20060102150405", ts)
	return err == nil
}
