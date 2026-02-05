package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

type PromResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string      `json:"resultType"`
		Result     interface{} `json:"result"`
	} `json:"data"`
}

type LabelValuesResponse struct {
	Status string   `json:"status"`
	Data   []string `json:"data"`
}

func main() {
	promURL := flag.String("prom-url", "http://localhost:9090", "Base URL of Prometheus")
	queryFile := flag.String("query-file", "queries.txt", "Path to file containing PromQL queries")
	rangeStr := flag.String("range", "30d", "Time range to query (e.g., 7d, 12h, 30m)")
	authHeader := flag.String("auth-header", "", "Authorization header for Prometheus API")
	step := flag.Duration("step", 5*time.Minute, "Step duration for the query")
	ticker := flag.Duration("ticker", 5*time.Second, "Interval between queries")
	labelName := flag.String("label", "instance", "Label name to fetch all values for")
	// numInstances := flag.Int("num-instances", 1, "1 instance - 20k")
	numThreads := flag.Int("num-threads", 1, "Number of threads to use for querying")
	sleep := flag.Duration("sleep", time.Millisecond, "Sleep duration before starting the queries")
	flag.Parse()

	t := time.NewTicker(*ticker)
	defer t.Stop()

	instances, err := getAllLabelValues(*promURL, *labelName, *authHeader)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error fetching label values: %v\n", err)
		os.Exit(1)
	}

	duration, err := parseDuration(*rangeStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid range: %v\n", err)
		os.Exit(1)
	}

	queries, err := loadQueries(*queryFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load queries: %v\n", err)
		os.Exit(1)
	}

	for {
		select {
		case <-t.C:
			end := time.Now()
			start := end.Add(-duration)
			var wg sync.WaitGroup
			for i := 0; i < *numThreads; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for i := 0; i < len(instances); i++ {
						for _, q := range queries {
							qWithInst := strings.ReplaceAll(q, `instance="host-0"`, fmt.Sprintf(`instance="%s"`, instances[i]))
							err := queryPrometheusRange(*promURL, qWithInst, start, end, *step, *authHeader)
							if err != nil {
								fmt.Fprintf(os.Stderr, "Instance %d query failed: %v\n", i, err)
							}
						}
						time.Sleep(*sleep)
					}
				}()
			}
			wg.Wait()
		}
	}
}

func parseDuration(s string) (time.Duration, error) {
	if strings.HasSuffix(s, "d") {
		days := strings.TrimSuffix(s, "d")
		n, err := time.ParseDuration(days + "h")
		if err != nil {
			return 0, err
		}
		return n * 24, nil
	}
	return time.ParseDuration(s)
}

func loadQueries(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var queries []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			queries = append(queries, line)
		}
	}
	return queries, scanner.Err()
}

func getAllLabelValues(baseURL, label string, authHeader string) ([]string, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	u, err := url.Parse(fmt.Sprintf("%s/api/v1/label/%s/values", baseURL, label))
	if err != nil {
		return nil, fmt.Errorf("failed to parse URL: %v", err)
	}

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}
	if authHeader != "" {
		req.Header.Set("Authorization", "Bearer "+authHeader)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %v", err)
	}

	defer resp.Body.Close()

	var res LabelValuesResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, body)
	}
	if res.Status != "success" {
		return nil, fmt.Errorf("failed to fetch label values")
	}

	return res.Data, nil
}

func queryPrometheusRange(baseURL, query string, start, end time.Time, step time.Duration, authHeader string) error {

	u, err := url.Parse(fmt.Sprintf("%s/api/v1/query_range", baseURL))
	if err != nil {
		return fmt.Errorf("failed to parse URL: %v", err)
	}

	q := u.Query()
	q.Set("query", query)
	q.Set("start", fmt.Sprintf("%d", start.Unix()))
	q.Set("end", fmt.Sprintf("%d", end.Unix()))
	q.Set("step", fmt.Sprintf("%.0f", step.Seconds()))
	u.RawQuery = q.Encode()

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return err
	}
	if authHeader != "" {
		req.Header.Set("Authorization", "Bearer "+authHeader)
	}
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, body)
	}

	var promResp PromResponse
	if err := json.NewDecoder(resp.Body).Decode(&promResp); err != nil {
		return fmt.Errorf("JSON decode error: %v", err)
	}
	return nil
}
