package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type result struct {
	status  int
	latency time.Duration
	body    string
}

func main() {
	baseURL := flag.String("url", "http://localhost:8080", "API base URL")
	requests := flag.Int("requests", 1000, "number of reservation requests")
	concurrency := flag.Int("concurrency", 100, "number of concurrent workers")
	seats := flag.Int("seats", 100, "event capacity")
	flag.Parse()

	client := &http.Client{Timeout: 5 * time.Second}
	eventID := fmt.Sprintf("load-%d", time.Now().UnixNano())
	createEvent(client, *baseURL, eventID, *seats)

	jobs := make(chan int)
	results := make(chan result, *requests)
	var completed atomic.Int64
	var wg sync.WaitGroup
	for worker := 0; worker < *concurrency; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for requestID := range jobs {
				started := time.Now()
				requestBody, _ := json.Marshal(map[string]string{"user_id": fmt.Sprintf("load-user-%d", requestID), "seat_id": fmt.Sprintf("seat-%d", requestID%*seats+1)})
				req, _ := http.NewRequest(http.MethodPost, *baseURL+"/v1/events/"+eventID+"/reservations", bytes.NewReader(requestBody))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Idempotency-Key", fmt.Sprintf("load-key-%d", requestID))
				resp, err := client.Do(req)
				status := http.StatusInternalServerError
				responseText := ""
				if err == nil {
					status = resp.StatusCode
					responseBody, _ := io.ReadAll(resp.Body)
					responseText = string(responseBody)
					resp.Body.Close()
				}
				results <- result{status: status, latency: time.Since(started), body: responseText}
				completed.Add(1)
			}
		}()
	}
	started := time.Now()
	go func() {
		for requestID := 0; requestID < *requests; requestID++ {
			jobs <- requestID
		}
		close(jobs)
	}()
	wg.Wait()
	close(results)

	latencies := make([]time.Duration, 0, *requests)
	statusCounts := make(map[int]int)
	errorBodies := make(map[string]int)
	for item := range results {
		latencies = append(latencies, item.latency)
		statusCounts[item.status]++
		if item.status != http.StatusCreated && item.status != http.StatusConflict {
			errorBodies[item.body]++
		}
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	if len(latencies) == 0 {
		panic("no requests completed")
	}
	fmt.Printf("requests=%d concurrency=%d duration=%s throughput=%.2f req/s\n", completed.Load(), *concurrency, time.Since(started).Round(time.Millisecond), float64(completed.Load())/time.Since(started).Seconds())
	fmt.Printf("p50=%s p95=%s p99=%s statuses=%v\n", percentile(latencies, 0.50), percentile(latencies, 0.95), percentile(latencies, 0.99), statusCounts)
	if len(errorBodies) > 0 {
		fmt.Printf("unexpected response bodies=%v\n", errorBodies)
	}
}

func createEvent(client *http.Client, baseURL, eventID string, seats int) {
	body, _ := json.Marshal(map[string]any{"event_id": eventID, "name": "Load test", "seat_count": seats})
	resp, err := client.Post(baseURL+"/v1/events", "application/json", bytes.NewReader(body))
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		panic(fmt.Sprintf("create event returned %s", resp.Status))
	}
}

func percentile(values []time.Duration, ratio float64) time.Duration {
	index := int(float64(len(values)-1) * ratio)
	return values[index]
}
