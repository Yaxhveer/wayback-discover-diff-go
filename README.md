# Wayback Discover Diff in Golang

## Overview
Wayback Discover Diff is a tool designed to calculate and return SimHash values for web archives stored in the Wayback Machine. This project is a reimplementation of the existing Python-based [wayback-discover-diff](https://github.com/internetarchive/wayback-discover-diff) using Golang for improved performance, better resource management, and enhanced maintainability.

The system leverages concurrent processing using Goroutines for job management, and optimized Redis queries. It also incorporates error handling, and retry mechanisms.

NOTE: Currently, this is just a prototype, and should not be used in production.

---

## Project Structure (Expected)
```
├───benchmark
│       main.go           
├───cmd
│       main.go          
└───internal
    ├───config
    │       config.go     
    ├───handlers
    │       handlers.go
    ├───job
    │       extract.go   
    │       extract_test.go
    │       job.go       
    │       job_test.go
    ├───simhash
    │       simhash.go  
    │       simhash_test.go
    ├───stats
    │       stats.go
    ├───tests
    │       e2e_test.go
    └───utils
            utils.go    
```

---

## Endpoints

### **1. Calculate SimHash for All Captures of a URL in a Year**
```
GET /calculate-simhash?url={URL}&year={YEAR}
```
- Checks if a job to calculate SimHash values is already running.
- If not, it creates a new job.
- **Returns:**
  - `{ "status": "started", "job_id": "XXYYZZ" }` if a new job is started.
  - `{ "status": "PENDING", "job_id": "XXYYZZ" }` if a job is already running.

---

## Key Features

1. **Efficient Job Management:**
    - Replaces Celery with Goroutines for concurrent task processing.
    - Ensures minimal overhead using Go's native concurrency model.

2. **Robust Error Handling:**
    - Implements retries with exponential backoff in case of connection errors.
    - Uses a pool of 20 workers to prevent connection refusal issues.

3. **Accurate SimHash Calculation:**
    - Golang-based implementation of SimHash for deduplication and similarity analysis.

4. **Logging:**
    - Detailed logs are generated for tracking progress and diagnosing issues.


---

### **2. Get SimHash for a Specific Capture**
```
GET /simhash?url={URL}&timestamp={TIMESTAMP}
```
- Returns the SimHash for a specific capture.
- **Returns:**
  - `{ "simhash": "XXXX" }` if the capture's SimHash exists.
  - `{ "message": "NO_CAPTURES", "status": "error" }` if no captures exist for the given URL and year.
  - `{ "message": "CAPTURE_NOT_FOUND", "status": "error" }` if the timestamp is invalid.

---

### **3. Get All SimHash Values for a Year**
```
GET /simhash?url={URL}&year={YEAR}
```
- Retrieves all timestamps and their corresponding SimHash values for a specific URL and year.
- **Returns:**
  - `["TIMESTAMP_VALUE", "SIMHASH_VALUE"]`
  - `{ "status": "error", "message": "NO_CAPTURES" }` if no captures exist.

---

### **4. Get Compressed SimHash Data**
```
GET /simhash?url={URL}&year={YEAR}&compress=1
```
- Provides a compressed data format for efficient retrieval.
- **Returns:**
  - `{ "captures": [...], "total_captures": XXX, "status": "COMPLETE" }` if data retrieval is complete.
  - `{ "status": "error", "message": "NO_CAPTURES" }` if no captures exist.

---

### **5. Paginated SimHash Results**
```
GET /simhash?url={URL}&year={YEAR}&page={PAGE_NUMBER}
```
- Returns paginated results of SimHash values.

---

### **6. Job Status**
```
GET /job?job_id={JOB_ID}
```
- Checks the status of a running SimHash job.
- **Returns:**
  - `{ "status": "pending", "job_id": "XXYYZZ", "info": "X out of Y captures have been processed" }`

---


## Installation
1. Clone the repository:
    ```bash
    git clone https://github.com/Yaxhveer/wayback-discover-diff-go.git
    cd wayback-discover-diff-go
    ```

2. Install dependencies:
    ```bash
    go mod tidy
    ```

3. Set up Redis and ensure it's running on your system.

4. Run the CLI:
    ```bash
    go run cmd/main.go
    ```
