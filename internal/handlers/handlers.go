package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"sync"

	"github.com/Yaxhveer/wayback-discover-diff-go/internal/job"
	"github.com/Yaxhveer/wayback-discover-diff-go/internal/utils"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

type Handler struct {
	redisClient *redis.Client
	jobsMap     map[string]*job.Job
	mu          sync.RWMutex
}

func NewHandler(redisClient *redis.Client) *Handler {
	return &Handler{
		redisClient: redisClient,
		jobsMap:     make(map[string]*job.Job),
	}
}

func getVersion() string {
	return "1.0.0"
}

// return job_id instead
func (h *Handler) getActiveTask(url, year string) *job.Job {
	h.mu.Lock()
	defer h.mu.Unlock()

	for _, j := range h.jobsMap {
		if j.URL == url && j.Year == year {
			return j
		}
	}
	return nil
}

func (h *Handler) Root(c *gin.Context) {
	version := getVersion()
	c.String(http.StatusOK, fmt.Sprintf("wayback-discover-diff service version: %s", version))
}

// GetSimhash fetches stored SimHash values for a given URL and optional timestamp/year
func (h *Handler) GetSimhash(c *gin.Context) {
	url := c.Query("url")
	if url == "" {
		c.IndentedJSON(http.StatusBadRequest, gin.H{"status": "error", "info": "url param is required."})
		return
	} else if !utils.URLIsValid(url) {
		c.IndentedJSON(http.StatusBadRequest, gin.H{"status": "error", "info": "invalid url format."})
		return
	}

	timestamp := c.Query("timestamp")

	if timestamp == "" {
		year := c.Query("year")
		if year == "" {
			c.IndentedJSON(http.StatusBadRequest, gin.H{"status": "error", "info": "year param is required."})
			return
		}
		var page int
		var err error

		pageStr := c.Query("page")
		if page, err = strconv.Atoi(pageStr); pageStr != "" && err != nil {
			page = -1
		}

		var snapshots_per_page int = -1 // from config

		resultStruct, err := utils.YearSimhash(h.redisClient, url, year, page, snapshots_per_page)
		if err != nil && len(resultStruct) == 0 {
			c.IndentedJSON(http.StatusAccepted, gin.H{
				"status":  "error",
				"message": err.Error(),
			})
			return
		}

		job := h.getActiveTask(url, year)
		status := "PENDING"
		if job != nil {
			status = job.State
		}

		compress := c.Query("compress")
		if compress == "true" || compress == "1" {
			captures, sortedHashes := utils.CompressCaptures(resultStruct)
			c.IndentedJSON(http.StatusOK, gin.H{
				"captures":       captures,
				"hashes":         sortedHashes,
				"total_captures": len(resultStruct),
				"status":         status,
			})
			return
		}

		c.IndentedJSON(http.StatusOK, gin.H{
			"captures":       resultStruct,
			"total_captures": len(resultStruct),
			"status":         status,
		})
		return
	}

	resultsMap, err := utils.TimestampSimHash(h.redisClient, url, timestamp)
	if err != nil {
		fmt.Printf("Cannot get simhash of url %s timestamp %s, %+v", url, timestamp, err)
		c.IndentedJSON(http.StatusAccepted, gin.H{
			"status":  "ERROR",
			"message": err.Error(),
		})
		return
	}
	job := h.getActiveTask(url, timestamp[:4])
	status := "PENDING"
	if job != nil {
		status = job.State
	}
	c.IndentedJSON(http.StatusOK, gin.H{
		"captures": resultsMap,
		"status":   status,
	})
}

// CalculateSimhash triggers a new SimHash calculation job
func (h *Handler) CalculateSimhash(c *gin.Context) {
	url := c.Query("url")
	if url == "" {
		c.IndentedJSON(http.StatusBadRequest, gin.H{"status": "error", "info": "url param is required."})
		return
	} else if !utils.URLIsValid(url) {
		c.IndentedJSON(http.StatusBadRequest, gin.H{"status": "error", "info": "invalid url format."})
		return
	}

	year := c.Query("year")
	if year == "" {
		c.IndentedJSON(http.StatusBadRequest, gin.H{"status": "error", "info": "year param is required."})
		return
	}

	task := h.getActiveTask(url, year)
	if task != nil && task.State == "PENDING" {
		c.IndentedJSON(http.StatusOK, gin.H{
			"status": "PENDING",
			"job_id": task.ID,
		})
		return
	}

	// added using config
	job := job.NewJobQueue()
	jobID := job.RunJob(*h.redisClient, url, year)

	h.mu.Lock()
	h.jobsMap[jobID] = job
	h.mu.Unlock()

	c.IndentedJSON(http.StatusAccepted, gin.H{
		"status": "STARTED",
		"job_id": jobID,
	})
}

func (h *Handler) GetJobStatus(c *gin.Context) {
	jobID := c.Query("job_id")
	if jobID == "" {
		c.IndentedJSON(http.StatusBadRequest, gin.H{"status": "error", "info": "job_id param is required."})
		return
	}

	h.mu.Lock()
	job, exists := h.jobsMap[jobID]
	h.mu.Unlock()

	if !exists {
		fmt.Printf("Cannot get job status of %s", jobID)
		c.IndentedJSON(http.StatusAccepted, gin.H{
			"status": "ERROR",
			"info":   "Cannot get status",
		})
		return
	}

	if job.State == "PENDING" || job.State == "ERROR" {
		c.IndentedJSON(http.StatusOK, gin.H{
			"status": job.State,
			"job_id": job.ID,
			"info":   job.Info,
		})
		return
	}

	c.IndentedJSON(http.StatusOK, gin.H{
		"state":    job.State,
		"job_id":   job.ID,
		"duration": job.Duration.Seconds(),
	})
}
